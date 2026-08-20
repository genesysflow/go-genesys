// Package event provides a static facade for the event dispatcher.
package event

import (
	"sync"

	"github.com/genesysflow/go-genesys/events"
)

var (
	instance *events.Dispatcher
	mu       sync.RWMutex
)

// SetInstance sets the dispatcher instance.
// This is called during application bootstrap.
func SetInstance(dispatcher *events.Dispatcher) {
	mu.Lock()
	defer mu.Unlock()
	instance = dispatcher
}

// GetInstance returns the dispatcher instance.
func GetInstance() *events.Dispatcher {
	mu.RLock()
	defer mu.RUnlock()
	return instance
}

func dispatcher() *events.Dispatcher {
	mu.RLock()
	defer mu.RUnlock()
	if instance == nil {
		panic("event: facade not initialised - register the EventServiceProvider")
	}
	return instance
}

// Dispatch dispatches an event implementing events.Event.
func Dispatch(event events.Event) error {
	return dispatcher().Dispatch(event)
}

// Listen registers a listener by event name.
func Listen(eventName string, listener events.Listener) {
	dispatcher().Listen(eventName, listener)
}

// Dispatcher returns the underlying dispatcher (for typed
// events.Listen/events.Emit helpers).
func Dispatcher() *events.Dispatcher {
	return dispatcher()
}
