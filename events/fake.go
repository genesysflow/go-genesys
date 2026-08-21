package events

import (
	"reflect"
)

// TestingT is the subset of *testing.T the assertion helpers need.
type TestingT interface {
	Helper()
	Errorf(format string, args ...any)
}

// Fake switches the dispatcher into recording mode - Laravel's
// Event::fake. Dispatched events are captured instead of invoking
// listeners, and the assertion helpers inspect what fired:
//
//	dispatcher.Fake()
//	...
//	dispatcher.AssertDispatched(t, "user.registered")
func (d *Dispatcher) Fake() *Dispatcher {
	d.fakeMu.Lock()
	defer d.fakeMu.Unlock()
	d.faked = true
	d.recorded = nil
	return d
}

// Unfake returns the dispatcher to normal delivery.
func (d *Dispatcher) Unfake() {
	d.fakeMu.Lock()
	defer d.fakeMu.Unlock()
	d.faked = false
}

// isFaked reports whether dispatches are being recorded, and records
// the event when so.
func (d *Dispatcher) recordIfFaked(event Event) bool {
	d.fakeMu.Lock()
	defer d.fakeMu.Unlock()
	if !d.faked {
		return false
	}
	d.recorded = append(d.recorded, event)
	return true
}

// Recorded returns the events captured while faked.
func (d *Dispatcher) Recorded() []Event {
	d.fakeMu.Lock()
	defer d.fakeMu.Unlock()
	return append([]Event(nil), d.recorded...)
}

// AssertDispatched asserts an event with the given name was dispatched
// while faked.
func (d *Dispatcher) AssertDispatched(t TestingT, name string) {
	t.Helper()
	for _, event := range d.Recorded() {
		if event.Name() == name {
			return
		}
	}
	t.Errorf("expected event %q to have been dispatched", name)
}

// AssertNotDispatched asserts no event with the given name fired.
func (d *Dispatcher) AssertNotDispatched(t TestingT, name string) {
	t.Helper()
	for _, event := range d.Recorded() {
		if event.Name() == name {
			t.Errorf("expected event %q not to have been dispatched", name)
			return
		}
	}
}

// AssertDispatchedCount asserts how many events with the name fired.
func (d *Dispatcher) AssertDispatchedCount(t TestingT, name string, expected int) {
	t.Helper()
	count := 0
	for _, event := range d.Recorded() {
		if event.Name() == name {
			count++
		}
	}
	if count != expected {
		t.Errorf("expected event %q to have been dispatched %d times, got %d", name, expected, count)
	}
}

// AssertNothingDispatched asserts no events fired at all.
func (d *Dispatcher) AssertNothingDispatched(t TestingT) {
	t.Helper()
	if recorded := d.Recorded(); len(recorded) > 0 {
		t.Errorf("expected no events, got %d (first: %s)", len(recorded), recorded[0].Name())
	}
}

// DispatchedOfType returns recorded events of the given concrete type.
func DispatchedOfType[T Event](d *Dispatcher) []T {
	var matches []T
	for _, event := range d.Recorded() {
		if typed, ok := event.(T); ok {
			matches = append(matches, typed)
		}
	}
	return matches
}

// AssertDispatchedEvent asserts an event of type T fired, optionally
// matching a predicate.
func AssertDispatchedEvent[T Event](t TestingT, d *Dispatcher, match ...func(T) bool) {
	t.Helper()
	for _, event := range DispatchedOfType[T](d) {
		if len(match) == 0 || match[0](event) {
			return
		}
	}
	t.Errorf("expected an event of type %s to have been dispatched", reflect.TypeOf((*T)(nil)).Elem())
}
