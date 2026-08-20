package events_test

import (
	"errors"
	"testing"

	"github.com/genesysflow/go-genesys/events"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// orderShipped does NOT implement events.Event: the name is derived from
// its type.
type orderShipped struct {
	OrderID int
}

// userRegistered implements events.Event with a custom name.
type userRegistered struct {
	Email string
}

func (e *userRegistered) Name() string { return "user.registered" }

func TestTypedListenAndEmit(t *testing.T) {
	d := events.NewDispatcher()

	var received []int
	events.Listen(d, func(e *orderShipped) error {
		received = append(received, e.OrderID)
		return nil
	})

	require.NoError(t, events.Emit(d, &orderShipped{OrderID: 7}))
	require.NoError(t, events.Emit(d, &orderShipped{OrderID: 8}))
	assert.Equal(t, []int{7, 8}, received)
}

func TestTypedListenWithNamedEvent(t *testing.T) {
	d := events.NewDispatcher()

	var emails []string
	events.Listen(d, func(e *userRegistered) error {
		emails = append(emails, e.Email)
		return nil
	})

	// Typed emit and interface dispatch both reach the listener.
	require.NoError(t, events.Emit(d, &userRegistered{Email: "a@x.io"}))
	require.NoError(t, d.Dispatch(&userRegistered{Email: "b@x.io"}))
	assert.Equal(t, []string{"a@x.io", "b@x.io"}, emails)
	assert.True(t, d.HasListeners("user.registered"))
}

func TestListenerErrorsPropagate(t *testing.T) {
	d := events.NewDispatcher()
	events.Listen(d, func(e *orderShipped) error {
		return errors.New("handler failed")
	})
	assert.ErrorContains(t, events.Emit(d, &orderShipped{}), "handler failed")
}
