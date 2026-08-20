package events_test

import (
	"fmt"
	"testing"

	"github.com/genesysflow/go-genesys/events"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type orderPlaced struct{ Total int }

func (e *orderPlaced) Name() string { return "order.placed" }

type recT struct{ failures []string }

func (r *recT) Helper() {}
func (r *recT) Errorf(format string, args ...any) {
	r.failures = append(r.failures, fmt.Sprintf(format, args...))
}

func TestFakeRecordsInsteadOfDelivering(t *testing.T) {
	dispatcher := events.NewDispatcher()

	delivered := false
	dispatcher.Listen("order.placed", func(e events.Event) error {
		delivered = true
		return nil
	})

	dispatcher.Fake()
	require.NoError(t, dispatcher.Dispatch(&orderPlaced{Total: 100}))
	assert.False(t, delivered, "faked dispatchers never invoke listeners")

	dispatcher.AssertDispatched(t, "order.placed")
	dispatcher.AssertDispatchedCount(t, "order.placed", 1)
	dispatcher.AssertNotDispatched(t, "order.cancelled")
	events.AssertDispatchedEvent[*orderPlaced](t, dispatcher, func(e *orderPlaced) bool {
		return e.Total == 100
	})
	assert.Len(t, events.DispatchedOfType[*orderPlaced](dispatcher), 1)

	// Unfake resumes delivery and stops recording.
	dispatcher.Unfake()
	require.NoError(t, dispatcher.Dispatch(&orderPlaced{Total: 5}))
	assert.True(t, delivered)
	dispatcher.AssertDispatchedCount(t, "order.placed", 1)
}

func TestFakeAssertionFailures(t *testing.T) {
	dispatcher := events.NewDispatcher().Fake()
	rec := &recT{}

	dispatcher.AssertDispatched(rec, "order.placed")
	require.NoError(t, dispatcher.Dispatch(&orderPlaced{}))
	dispatcher.AssertNotDispatched(rec, "order.placed")
	dispatcher.AssertNothingDispatched(rec)
	dispatcher.AssertDispatchedCount(rec, "order.placed", 3)

	assert.Len(t, rec.failures, 4)
}
