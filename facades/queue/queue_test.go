package queue_test

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/genesysflow/go-genesys/facades/queue"
	basequeue "github.com/genesysflow/go-genesys/queue"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var handled atomic.Int64

type pingJob struct{}

func (j *pingJob) Handle() error {
	handled.Add(1)
	return nil
}

func TestDispatchThroughFacade(t *testing.T) {
	handled.Store(0)
	queue.SetInstance(basequeue.NewManager()) // default connection: sync
	t.Cleanup(func() { queue.SetInstance(nil) })

	require.NoError(t, queue.Dispatch(&pingJob{}))
	assert.EqualValues(t, 1, handled.Load())

	// The sync driver runs delayed jobs immediately.
	require.NoError(t, queue.DispatchLater(time.Hour, &pingJob{}))
	assert.EqualValues(t, 2, handled.Load())

	assert.NotNil(t, queue.Connection())
	assert.NotNil(t, queue.GetInstance())
}

func TestFacadePanics(t *testing.T) {
	queue.SetInstance(nil)
	assert.Panics(t, func() { queue.Dispatch(&pingJob{}) })

	queue.SetInstance(basequeue.NewManager())
	t.Cleanup(func() { queue.SetInstance(nil) })
	assert.Panics(t, func() { queue.Connection("missing") })
}
