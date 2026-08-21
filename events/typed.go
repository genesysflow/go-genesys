package events

import (
	"fmt"
	"reflect"
)

// NameOf resolves an event's name: its Name() when it implements Event,
// otherwise the reflect-derived "pkgpath.TypeName".
func NameOf(event any) string {
	if named, ok := event.(Event); ok {
		return named.Name()
	}
	t := reflect.TypeOf(event)
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.PkgPath() != "" {
		return t.PkgPath() + "." + t.Name()
	}
	return t.Name()
}

// typeName resolves the name a type registers under without an instance.
func typeName[T any]() string {
	var probe T
	return NameOf(&probe)
}

// Listen registers a typed listener; the event type is inferred and the
// type assertion is handled for you:
//
//	events.Listen(dispatcher, func(e *UserRegistered) error {
//	    return sendWelcomeEmail(e.User)
//	})
func Listen[T any](d *Dispatcher, listener func(event *T) error) {
	d.Listen(typeName[T](), func(event Event) error {
		typed, ok := any(event).(*typedEvent[T])
		if ok {
			return listener(typed.payload)
		}
		// Events implementing Event and dispatched directly.
		if direct, ok := any(event).(*T); ok {
			return listener(direct)
		}
		return fmt.Errorf("events: listener for %s received %T", typeName[T](), event)
	})
}

// typedEvent adapts an arbitrary struct to the Event interface.
type typedEvent[T any] struct {
	payload *T
	name    string
}

func (e *typedEvent[T]) Name() string { return e.name }

// Emit dispatches a typed event; the struct does not need to implement
// Event:
//
//	events.Emit(dispatcher, &UserRegistered{User: user})
func Emit[T any](d *Dispatcher, event *T) error {
	if asEvent, ok := any(event).(Event); ok {
		return d.Dispatch(asEvent)
	}
	return d.Dispatch(&typedEvent[T]{payload: event, name: NameOf(event)})
}
