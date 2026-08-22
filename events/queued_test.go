package events_test

import (
	"sync/atomic"
	"testing"

	"github.com/genesysflow/go-genesys/events"
	"github.com/genesysflow/go-genesys/queue"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type queuedOrderShipped struct {
	OrderID int64 `json:"order_id"`
}

func (queuedOrderShipped) Name() string { return "order.shipped" }

func TestQueuedListenerReplaysOnWorker(t *testing.T) {
	d := events.NewDispatcher()
	q := queue.NewMemoryQueue()

	var handled atomic.Int64
	events.ListenQueued(d, q, "receipts.send", func(e queuedOrderShipped) error {
		handled.Store(e.OrderID)
		return nil
	})

	// Dispatch queues the work instead of running it inline.
	require.NoError(t, d.Dispatch(queuedOrderShipped{OrderID: 42}))
	assert.EqualValues(t, 0, handled.Load(), "nothing runs until a worker drains")
	assertQueued(t, q, 1)

	require.NoError(t, queue.NewWorker(q).Drain())
	assert.EqualValues(t, 42, handled.Load(), "the worker replayed the event with its payload")
}

type auditSubscriber struct{ log *[]string }

func (s auditSubscriber) Subscribe(d *events.Dispatcher) {
	d.Listen("user.login", func(e events.Event) error {
		*s.log = append(*s.log, "login")
		return nil
	})
	d.Listen("user.logout", func(e events.Event) error {
		*s.log = append(*s.log, "logout")
		return nil
	})
}

type namedEvent string

func (e namedEvent) Name() string { return string(e) }

func TestSubscriberRegistersItsListeners(t *testing.T) {
	d := events.NewDispatcher()
	var log []string
	d.Subscribe(auditSubscriber{log: &log})

	require.NoError(t, d.Dispatch(namedEvent("user.login")))
	require.NoError(t, d.Dispatch(namedEvent("user.logout")))
	assert.Equal(t, []string{"login", "logout"}, log)
}

// assertQueued checks how many jobs are waiting, failing on a driver
// error rather than reading it as an empty queue.
func assertQueued(t *testing.T, q *queue.MemoryQueue, expected int64) {
	t.Helper()
	size, err := q.Size("")
	require.NoError(t, err)
	assert.Equal(t, expected, size)
}
