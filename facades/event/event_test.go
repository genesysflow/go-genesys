package event_test

import (
	"testing"

	"github.com/genesysflow/go-genesys/events"
	"github.com/genesysflow/go-genesys/facades/event"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type demoEvent struct{ N int }

func (e *demoEvent) Name() string { return "demo" }

func TestFacadeDispatch(t *testing.T) {
	event.SetInstance(events.NewDispatcher())
	t.Cleanup(func() { event.SetInstance(nil) })

	var got int
	event.Listen("demo", func(e events.Event) error {
		got = e.(*demoEvent).N
		return nil
	})
	require.NoError(t, event.Dispatch(&demoEvent{N: 9}))
	assert.Equal(t, 9, got)

	// Typed helpers work against the facade's dispatcher.
	events.Listen(event.Dispatcher(), func(e *demoEvent) error {
		got = e.N * 2
		return nil
	})
	require.NoError(t, events.Emit(event.Dispatcher(), &demoEvent{N: 10}))
	assert.Equal(t, 20, got)

	assert.NotNil(t, event.GetInstance())
}

func TestFacadePanicsWithoutInstance(t *testing.T) {
	event.SetInstance(nil)
	assert.Panics(t, func() { event.Dispatch(&demoEvent{}) })
}
