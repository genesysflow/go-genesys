package events

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/genesysflow/go-genesys/queue"
)

// Subscriber groups several listener registrations in one place -
// Laravel's event subscribers:
//
//	type UserEventSubscriber struct{}
//	func (s UserEventSubscriber) Subscribe(d *events.Dispatcher) {
//	    d.Listen("user.login", s.onLogin)
//	    d.Listen("user.logout", s.onLogout)
//	}
//
//	dispatcher.Subscribe(UserEventSubscriber{})
type Subscriber interface {
	Subscribe(d *Dispatcher)
}

// Subscribe registers each subscriber's listeners.
func (d *Dispatcher) Subscribe(subscribers ...Subscriber) {
	for _, s := range subscribers {
		s.Subscribe(d)
	}
}

// --- queued listeners ---

var (
	queuedHandlersMu sync.RWMutex
	queuedHandlers   = map[string]func(payload json.RawMessage) error{}
)

// queuedListenerJob replays a serialized event through a registered
// handler on a queue worker.
type queuedListenerJob struct {
	Handler string          `json:"handler"`
	Payload json.RawMessage `json:"payload"`
}

// JobName gives the wrapper a stable registered name.
func (j *queuedListenerJob) JobName() string { return "genesys.listener" }

// Handle looks up the named handler and replays the event into it.
func (j *queuedListenerJob) Handle() error {
	queuedHandlersMu.RLock()
	handler, ok := queuedHandlers[j.Handler]
	queuedHandlersMu.RUnlock()
	if !ok {
		return fmt.Errorf("events: queued listener %q is not registered - call events.ListenQueued in the worker process too", j.Handler)
	}
	return handler(j.Payload)
}

func init() {
	queue.Register[queuedListenerJob]()
}

// ListenQueued registers a listener that handles its events on the
// queue - Laravel's ShouldQueue listeners. Dispatching the event pushes
// a job carrying the JSON-serialized event; a worker replays it through
// the handler. The worker process must call ListenQueued with the same
// name so the handler is registered there too. Event types must
// round-trip through JSON:
//
//	events.ListenQueued(dispatcher, q, "receipts.send", func(e OrderShipped) error {
//	    return sendReceipt(e.OrderID)
//	})
func ListenQueued[E Event](d *Dispatcher, q queue.Queue, name string, handler func(E) error) {
	queuedHandlersMu.Lock()
	queuedHandlers[name] = func(payload json.RawMessage) error {
		var event E
		if err := json.Unmarshal(payload, &event); err != nil {
			return fmt.Errorf("events: queued listener %q: cannot deserialize event: %w", name, err)
		}
		return handler(event)
	}
	queuedHandlersMu.Unlock()

	var probe E
	d.Listen(probe.Name(), func(event Event) error {
		payload, err := json.Marshal(event)
		if err != nil {
			return fmt.Errorf("events: queued listener %q: cannot serialize event: %w", name, err)
		}
		return q.Push(&queuedListenerJob{Handler: name, Payload: payload})
	})
}
