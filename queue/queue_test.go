package queue_test

import (
	"errors"
	"testing"

	"github.com/genesysflow/go-genesys/queue"

	"github.com/stretchr/testify/assert"
)

// MockJob is a mock implementation of the Job interface.
type MockJob struct {
	executed bool
	err      error
}

func (j *MockJob) Handle() error {
	j.executed = true
	return j.err
}

func TestSyncQueue(t *testing.T) {
	t.Run("it executes job immediately", func(t *testing.T) {
		q := queue.NewSyncQueue()
		job := &MockJob{}

		err := q.Push(job)

		assert.NoError(t, err)
		assert.True(t, job.executed)
	})

	t.Run("it returns error from job", func(t *testing.T) {
		q := queue.NewSyncQueue()
		expectedErr := errors.New("job failed")
		job := &MockJob{err: expectedErr}

		err := q.Push(job)

		assert.Equal(t, expectedErr, err)
		assert.True(t, job.executed)
	})
}

func TestManager(t *testing.T) {
	t.Run("it returns sync connection by default", func(t *testing.T) {
		manager := queue.NewManager()

		conn, err := manager.Connection()

		assert.NoError(t, err)
		assert.IsType(t, &queue.SyncQueue{}, conn)
	})

	t.Run("it returns specific connection", func(t *testing.T) {
		manager := queue.NewManager()

		// Should resolve default "sync" when asked explicitly
		conn, err := manager.Connection("sync")
		assert.NoError(t, err)
		assert.IsType(t, &queue.SyncQueue{}, conn)
	})

	t.Run("it returns error for unknown connection", func(t *testing.T) {
		manager := queue.NewManager()

		conn, err := manager.Connection("unknown")

		assert.Error(t, err)
		assert.Nil(t, conn)
		assert.Contains(t, err.Error(), "queue connection [unknown] not found")
	})

	t.Run("it registers and retrieves custom connection", func(t *testing.T) {
		manager := queue.NewManager()
		customQueue := queue.NewSyncQueue()

		manager.Register("custom", customQueue)
		conn, err := manager.Connection("custom")

		assert.NoError(t, err)
		assert.Equal(t, customQueue, conn)
	})

	t.Run("it reuses connections", func(t *testing.T) {
		manager := queue.NewManager()

		conn1, _ := manager.Connection("sync")
		conn2, _ := manager.Connection("sync")

		assert.Equal(t, conn1, conn2)
	})
}
