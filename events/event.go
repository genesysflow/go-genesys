package events

// Event is the interface that all events must implement.
type Event interface {
	// Name returns the event name.
	Name() string
}
