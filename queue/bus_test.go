package queue_test

import (
	"errors"
	"sync"
	"testing"

	"github.com/genesysflow/go-genesys/queue"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	busMu  sync.Mutex
	busLog []string
)

func logStep(step string) {
	busMu.Lock()
	defer busMu.Unlock()
	busLog = append(busLog, step)
}

func readLog() []string {
	busMu.Lock()
	defer busMu.Unlock()
	return append([]string(nil), busLog...)
}

func resetLog() {
	busMu.Lock()
	defer busMu.Unlock()
	busLog = nil
}

type stepJob struct {
	Step string
	Fail bool
}

func (j *stepJob) Handle() error {
	logStep(j.Step)
	if j.Fail {
		return errors.New("step " + j.Step + " failed")
	}
	return nil
}

func TestChainRunsSequentiallyOnWorker(t *testing.T) {
	queue.Register[stepJob]()
	resetLog()

	q := queue.NewMemoryQueue()
	require.NoError(t, queue.Chain(q,
		&stepJob{Step: "one"},
		&stepJob{Step: "two"},
		&stepJob{Step: "three"},
	))

	// Only the chain head is on the queue; each link enqueues the next.
	assert.Equal(t, 1, q.Size(""))

	worker := queue.NewWorker(q)
	require.NoError(t, worker.Drain())
	assert.Equal(t, []string{"one", "two", "three"}, readLog())
}

func TestChainStopsOnFailure(t *testing.T) {
	queue.Register[stepJob]()
	resetLog()

	q := queue.NewMemoryQueue()
	require.NoError(t, queue.Chain(q,
		&stepJob{Step: "one"},
		&stepJob{Step: "boom", Fail: true},
		&stepJob{Step: "never"},
	))

	worker := queue.NewWorker(q)
	worker.Tries = 1
	require.NoError(t, worker.Drain())

	log := readLog()
	assert.Contains(t, log, "one")
	assert.Contains(t, log, "boom")
	assert.NotContains(t, log, "never", "links after a failure never run")

	// The failed chain landed on the failed list.
	failed, err := q.ListFailed()
	require.NoError(t, err)
	require.Len(t, failed, 1)
	assert.Equal(t, "genesys.chain", failed[0].Name)
}

func TestChainOnSyncQueueRunsInline(t *testing.T) {
	queue.Register[stepJob]()
	resetLog()

	q := queue.NewSyncQueue()
	require.NoError(t, queue.Chain(q, &stepJob{Step: "a"}, &stepJob{Step: "b"}))
	assert.Equal(t, []string{"a", "b"}, readLog())
}

func TestBatchCallbacks(t *testing.T) {
	queue.Register[stepJob]()
	resetLog()

	q := queue.NewMemoryQueue()

	var thenCalled, finallyCalled bool
	batch := queue.NewBatch().
		Then(func(b *queue.Batch) { thenCalled = true }).
		Finally(func(b *queue.Batch) { finallyCalled = true })

	require.NoError(t, batch.Dispatch(q,
		&stepJob{Step: "b1"},
		&stepJob{Step: "b2"},
		&stepJob{Step: "b3"},
	))
	assert.Equal(t, 3, q.Size(""))

	settled, total := batch.Progress()
	assert.Equal(t, 0, settled)
	assert.Equal(t, 3, total)
	assert.False(t, batch.Finished())

	worker := queue.NewWorker(q)
	require.NoError(t, worker.Drain())

	assert.ElementsMatch(t, []string{"b1", "b2", "b3"}, readLog())
	assert.True(t, batch.Finished())
	assert.True(t, thenCalled, "Then fires when everything succeeded")
	assert.True(t, finallyCalled)
	assert.Equal(t, 0, batch.FailedCount())
}

func TestBatchCatchOnFailure(t *testing.T) {
	queue.Register[stepJob]()
	resetLog()

	q := queue.NewMemoryQueue()

	var thenCalled bool
	var caught error
	batch := queue.NewBatch().
		Then(func(b *queue.Batch) { thenCalled = true }).
		Catch(func(b *queue.Batch, err error) { caught = err })

	require.NoError(t, batch.Dispatch(q,
		&stepJob{Step: "ok"},
		&stepJob{Step: "bad", Fail: true},
	))

	worker := queue.NewWorker(q)
	worker.Tries = 1
	require.NoError(t, worker.Drain())

	assert.True(t, batch.Finished())
	assert.False(t, thenCalled, "Then must not fire when a job failed")
	require.Error(t, caught)
	assert.Contains(t, caught.Error(), "bad")
	assert.Equal(t, 1, batch.FailedCount())
}

func TestBatchOnSyncQueue(t *testing.T) {
	queue.Register[stepJob]()
	resetLog()

	q := queue.NewSyncQueue()
	done := false
	batch := queue.NewBatch().Then(func(b *queue.Batch) { done = true })

	// Sync queues run each job at Push time; a failing job surfaces
	// its error from Dispatch.
	require.NoError(t, batch.Dispatch(q, &stepJob{Step: "s1"}, &stepJob{Step: "s2"}))
	assert.Equal(t, []string{"s1", "s2"}, readLog())
	assert.True(t, done)
}
