package events

import (
	"sync"
)

// Listener is the function signature for event listeners.
type Listener func(event Event) error

// Dispatcher manages event listeners and dispatching.
type Dispatcher struct {
	listeners map[string][]Listener
	wildcards []Listener
	mu        sync.RWMutex

	// fake-mode state (see fake.go)
	fakeMu   sync.Mutex
	faked    bool
	recorded []Event
}

// NewDispatcher creates a new event dispatcher.
func NewDispatcher() *Dispatcher {
	return &Dispatcher{
		listeners: make(map[string][]Listener),
	}
}

// Listen registers a listener for an event.
func (d *Dispatcher) Listen(eventName string, listener Listener) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.listeners[eventName] = append(d.listeners[eventName], listener)
}

// ListenAll registers a wildcard listener invoked for every dispatched
// event, after the event's own listeners. Used by cross-cutting
// consumers such as the broadcasting bridge.
func (d *Dispatcher) ListenAll(listener Listener) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.wildcards = append(d.wildcards, listener)
}

// Dispatch dispatches an event to all registered listeners. While the
// dispatcher is faked (see Fake) the event is recorded instead.
func (d *Dispatcher) Dispatch(event Event) error {
	if d.recordIfFaked(event) {
		return nil
	}
	d.mu.RLock()
	listeners := d.listeners[event.Name()]
	wildcards := d.wildcards
	d.mu.RUnlock()

	for _, listener := range listeners {
		if err := listener(event); err != nil {
			return err
		}
	}
	for _, listener := range wildcards {
		if err := listener(event); err != nil {
			return err
		}
	}
	return nil
}

// HasListeners checks if an event has listeners.
func (d *Dispatcher) HasListeners(eventName string) bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return len(d.listeners[eventName]) > 0
}

// Forget removes all listeners for an event.
func (d *Dispatcher) Forget(eventName string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.listeners, eventName)
}
