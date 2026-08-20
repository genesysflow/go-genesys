package broadcast

import (
	"github.com/genesysflow/go-genesys/events"
)

// Broadcastable events are pushed to a channel when dispatched -
// Laravel's ShouldBroadcast:
//
//	type OrderShipped struct{ ID int64 }
//
//	func (e *OrderShipped) Name() string             { return "order.shipped" }
//	func (e *OrderShipped) BroadcastChannel() string { return "orders" }
type Broadcastable interface {
	events.Event

	// BroadcastChannel names the channel the event goes out on.
	BroadcastChannel() string
}

// HasBroadcastPayload overrides the broadcast payload (defaults to the
// JSON encoding of the event itself).
type HasBroadcastPayload interface {
	BroadcastPayload() any
}

// HasBroadcastName overrides the event name sent to clients (defaults
// to the event's Name()).
type HasBroadcastName interface {
	BroadcastAs() string
}

// ConnectDispatcher wires a dispatcher to the hub: every dispatched
// event implementing Broadcastable is pushed to its channel.
func ConnectDispatcher(dispatcher *events.Dispatcher, hub *Hub) {
	dispatcher.ListenAll(func(event events.Event) error {
		broadcastable, ok := event.(Broadcastable)
		if !ok {
			return nil
		}
		name := broadcastable.Name()
		if named, ok := event.(HasBroadcastName); ok {
			name = named.BroadcastAs()
		}
		var payload any = event
		if withPayload, ok := event.(HasBroadcastPayload); ok {
			payload = withPayload.BroadcastPayload()
		}
		return hub.Broadcast(broadcastable.BroadcastChannel(), name, payload)
	})
}
