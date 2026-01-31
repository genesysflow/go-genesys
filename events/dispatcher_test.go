package events

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testEvent implements the Event interface for testing.
type testEvent struct {
	name    string
	payload any
}

func (e *testEvent) Name() string {
	return e.name
}

func (e *testEvent) Payload() any {
	return e.payload
}

func newTestEvent(name string, payload any) *testEvent {
	return &testEvent{name: name, payload: payload}
}

func TestNewDispatcher(t *testing.T) {
	d := NewDispatcher()
	assert.NotNil(t, d)
	assert.NotNil(t, d.listeners)
}

func TestListen(t *testing.T) {
	d := NewDispatcher()

	d.Listen("test.event", func(event Event) error {
		return nil
	})

	assert.True(t, d.HasListeners("test.event"))
	assert.False(t, d.HasListeners("other.event"))
}

func TestDispatch(t *testing.T) {
	d := NewDispatcher()

	var receivedPayload any
	d.Listen("user.created", func(event Event) error {
		if te, ok := event.(*testEvent); ok {
			receivedPayload = te.Payload()
		}
		return nil
	})

	err := d.Dispatch(newTestEvent("user.created", "test-payload"))
	require.NoError(t, err)
	assert.Equal(t, "test-payload", receivedPayload)
}

func TestDispatchMultipleListeners(t *testing.T) {
	d := NewDispatcher()

	var callOrder []int
	d.Listen("event", func(event Event) error {
		callOrder = append(callOrder, 1)
		return nil
	})
	d.Listen("event", func(event Event) error {
		callOrder = append(callOrder, 2)
		return nil
	})
	d.Listen("event", func(event Event) error {
		callOrder = append(callOrder, 3)
		return nil
	})

	err := d.Dispatch(newTestEvent("event", nil))
	require.NoError(t, err)
	assert.Equal(t, []int{1, 2, 3}, callOrder)
}

func TestDispatchNoListeners(t *testing.T) {
	d := NewDispatcher()

	// Should not error when dispatching to event with no listeners
	err := d.Dispatch(newTestEvent("no.listeners", nil))
	require.NoError(t, err)
}

func TestDispatchListenerError(t *testing.T) {
	d := NewDispatcher()

	expectedErr := errors.New("listener error")
	d.Listen("error.event", func(event Event) error {
		return expectedErr
	})

	err := d.Dispatch(newTestEvent("error.event", nil))
	assert.Equal(t, expectedErr, err)
}

func TestDispatchStopsOnError(t *testing.T) {
	d := NewDispatcher()

	secondCalled := false
	d.Listen("event", func(event Event) error {
		return errors.New("first listener error")
	})
	d.Listen("event", func(event Event) error {
		secondCalled = true
		return nil
	})

	err := d.Dispatch(newTestEvent("event", nil))
	assert.Error(t, err)
	assert.False(t, secondCalled, "Second listener should not be called when first errors")
}

func TestHasListeners(t *testing.T) {
	d := NewDispatcher()

	assert.False(t, d.HasListeners("test"))

	d.Listen("test", func(event Event) error { return nil })

	assert.True(t, d.HasListeners("test"))
	assert.False(t, d.HasListeners("other"))
}

func TestForget(t *testing.T) {
	d := NewDispatcher()

	d.Listen("test", func(event Event) error { return nil })
	assert.True(t, d.HasListeners("test"))

	d.Forget("test")
	assert.False(t, d.HasListeners("test"))
}

func TestForgetNonExistent(t *testing.T) {
	d := NewDispatcher()

	// Should not panic when forgetting non-existent event
	d.Forget("nonexistent")
	assert.False(t, d.HasListeners("nonexistent"))
}

func TestDispatchWithComplexPayload(t *testing.T) {
	d := NewDispatcher()

	type UserData struct {
		ID    int
		Name  string
		Email string
	}

	var received UserData
	d.Listen("user.created", func(event Event) error {
		if te, ok := event.(*testEvent); ok {
			if payload, ok := te.Payload().(UserData); ok {
				received = payload
			}
		}
		return nil
	})

	user := UserData{ID: 1, Name: "John", Email: "john@example.com"}
	err := d.Dispatch(newTestEvent("user.created", user))
	require.NoError(t, err)
	assert.Equal(t, user, received)
}

func TestConcurrentDispatch(t *testing.T) {
	d := NewDispatcher()

	var counter int64
	d.Listen("concurrent.event", func(event Event) error {
		atomic.AddInt64(&counter, 1)
		return nil
	})

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			d.Dispatch(newTestEvent("concurrent.event", nil))
		}()
	}
	wg.Wait()

	assert.Equal(t, int64(100), counter)
}

func TestConcurrentListenAndDispatch(t *testing.T) {
	d := NewDispatcher()

	var wg sync.WaitGroup

	// Concurrent listeners
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			d.Listen("concurrent", func(event Event) error { return nil })
		}()
	}

	// Concurrent dispatches
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			d.Dispatch(newTestEvent("concurrent", nil))
		}()
	}

	wg.Wait()
	assert.True(t, d.HasListeners("concurrent"))
}

func TestMultipleEventTypes(t *testing.T) {
	d := NewDispatcher()

	var results []string
	d.Listen("event.a", func(event Event) error {
		results = append(results, "a")
		return nil
	})
	d.Listen("event.b", func(event Event) error {
		results = append(results, "b")
		return nil
	})
	d.Listen("event.c", func(event Event) error {
		results = append(results, "c")
		return nil
	})

	d.Dispatch(newTestEvent("event.b", nil))
	d.Dispatch(newTestEvent("event.a", nil))
	d.Dispatch(newTestEvent("event.c", nil))

	assert.Equal(t, []string{"b", "a", "c"}, results)
}
