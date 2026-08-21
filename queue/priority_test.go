package queue_test

import (
	"sync"
	"testing"
	"time"

	"github.com/genesysflow/go-genesys/queue"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type priorityJob struct{ Label string }

var (
	priorityMu  sync.Mutex
	priorityLog []string
)

func (j *priorityJob) Handle() error {
	priorityMu.Lock()
	defer priorityMu.Unlock()
	priorityLog = append(priorityLog, j.Label)
	return nil
}

func TestPushOnAndWorkerPriorities(t *testing.T) {
	queue.Register[priorityJob]()
	priorityMu.Lock()
	priorityLog = nil
	priorityMu.Unlock()

	q := queue.NewMemoryQueue()

	// Interleave dispatches across queues; priority order must win.
	require.NoError(t, queue.PushOn(q, "default", &priorityJob{Label: "d1"}))
	require.NoError(t, queue.PushOn(q, "high", &priorityJob{Label: "h1"}))
	require.NoError(t, queue.PushOn(q, "default", &priorityJob{Label: "d2"}))
	require.NoError(t, queue.PushOn(q, "high", &priorityJob{Label: "h2"}))

	assert.Equal(t, 2, q.Size("high"))
	assert.Equal(t, 2, q.Size("default"))

	worker := queue.NewWorker(q)
	worker.Queues = []string{"high", "default"}
	require.NoError(t, worker.Drain())

	assert.Equal(t, []string{"h1", "h2", "d1", "d2"}, priorityLog,
		"high drains completely before default")
	assert.Equal(t, 0, q.Size("high"))
	assert.Equal(t, 0, q.Size("default"))
}

func TestLaterOnDelays(t *testing.T) {
	queue.Register[priorityJob]()
	q := queue.NewMemoryQueue()

	require.NoError(t, queue.LaterOn(q, "reports", time.Hour, &priorityJob{Label: "later"}))
	job, err := q.Pop("reports")
	require.NoError(t, err)
	assert.Nil(t, job, "not due yet")
	assert.Equal(t, 1, q.Size("reports"))
}

func TestPushOnSyncDriverRunsInline(t *testing.T) {
	queue.Register[priorityJob]()
	priorityMu.Lock()
	priorityLog = nil
	priorityMu.Unlock()

	// Like Laravel's sync connection, onQueue is a no-op: the job runs
	// immediately instead of erroring, so dev/test setups keep working.
	require.NoError(t, queue.PushOn(queue.NewSyncQueue(), "high", &priorityJob{Label: "inline"}))

	priorityMu.Lock()
	defer priorityMu.Unlock()
	assert.Equal(t, []string{"inline"}, priorityLog)
}

func TestRedisPushOnNamedQueues(t *testing.T) {
	q, _ := newRedisQueue(t)

	require.NoError(t, queue.PushOn(q, "high", &redisTestJob{Word: "urgent"}))
	require.NoError(t, queue.PushOn(q, "", &redisTestJob{Word: "normal"}))

	assert.Equal(t, 1, q.Size("high"))
	assert.Equal(t, 1, q.Size("default"))

	job, err := q.Pop("high")
	require.NoError(t, err)
	require.NotNil(t, job)
	assert.Equal(t, "high", job.Queue)
}
