package queue_test

import (
	"errors"
	"testing"
	"time"

	"github.com/genesysflow/go-genesys/queue"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSyncQueueLaterRunsImmediately(t *testing.T) {
	resetJobState()
	q := queue.NewSyncQueue()
	require.NoError(t, q.Later(time.Hour, &CountJob{}))
	assert.EqualValues(t, 1, handled.Load(), "sync driver ignores delays")
}

func TestManagerBuiltInAndLazyConnections(t *testing.T) {
	manager := queue.NewManager()

	// Built-in drivers resolve by name.
	memoryConn, err := manager.Connection("memory")
	require.NoError(t, err)
	assert.IsType(t, &queue.MemoryQueue{}, memoryConn)

	// Lazy factories build once and are cached.
	builds := 0
	manager.RegisterLazy("custom", func() (queue.Queue, error) {
		builds++
		return queue.NewMemoryQueue(), nil
	})
	first, err := manager.Connection("custom")
	require.NoError(t, err)
	second, err := manager.Connection("custom")
	require.NoError(t, err)
	assert.Same(t, first, second)
	assert.Equal(t, 1, builds)

	// A failing factory propagates its error.
	manager.RegisterLazy("broken", func() (queue.Queue, error) {
		return nil, errors.New("cannot connect")
	})
	_, err = manager.Connection("broken")
	assert.ErrorContains(t, err, "cannot connect")

	// Default connection switching.
	assert.Equal(t, "sync", manager.DefaultConnection())
	manager.SetDefaultConnection("memory")
	assert.Equal(t, "memory", manager.DefaultConnection())
	conn, err := manager.Connection()
	require.NoError(t, err)
	assert.IsType(t, &queue.MemoryQueue{}, conn)
}

// unregisteredJob is never registered with the queue registry.
type unregisteredJob struct{}

func (j *unregisteredJob) Handle() error { return nil }

func TestUnregisteredJobFailsAtWorker(t *testing.T) {
	q := queue.NewMemoryQueue()
	require.NoError(t, q.Push(&unregisteredJob{}))

	worker := queue.NewWorker(q)
	require.NoError(t, worker.Drain())

	failed, err := q.ListFailed()
	require.NoError(t, err)
	require.Len(t, failed, 1)
	assert.Contains(t, failed[0].Exception, "not registered")

	// ForgetFailed removes it without retrying.
	require.NoError(t, q.ForgetFailed(failed[0].ID))
	failed, err = q.ListFailed()
	require.NoError(t, err)
	assert.Empty(t, failed)
}

// namedJob overrides its registered name.
type namedJob struct{ Payload string }

func (j *namedJob) Handle() error   { return nil }
func (j *namedJob) JobName() string { return "custom.named" }

func TestNameableJobs(t *testing.T) {
	queue.Register[namedJob]()
	q := queue.NewMemoryQueue()
	require.NoError(t, q.Push(&namedJob{Payload: "x"}))

	reserved, err := q.Pop("")
	require.NoError(t, err)
	require.NotNil(t, reserved)
	assert.Equal(t, "custom.named", reserved.Name)
}
