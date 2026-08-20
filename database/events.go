package database

import (
	"reflect"
	"sync"
)

// Model lifecycle hooks, Eloquent's model events. A model opts in by
// implementing any of these on its pointer receiver:
//
//	func (u *User) Creating() error { u.Slug = slugify(u.Name); return nil }
//
// Returning an error aborts the operation. Firing order matches Laravel:
// Saving, Creating -> INSERT -> Created, Saved (and the Updating twin for
// updates). Deleting/Deleted fire around DeleteModel.
type (
	// CreatingHook runs before a model is inserted.
	CreatingHook interface{ Creating() error }
	// CreatedHook runs after a model is inserted.
	CreatedHook interface{ Created() error }
	// UpdatingHook runs before a model is updated.
	UpdatingHook interface{ Updating() error }
	// UpdatedHook runs after a model is updated.
	UpdatedHook interface{ Updated() error }
	// SavingHook runs before a model is inserted or updated.
	SavingHook interface{ Saving() error }
	// SavedHook runs after a model is inserted or updated.
	SavedHook interface{ Saved() error }
	// DeletingHook runs before a model is deleted via DeleteModel.
	DeletingHook interface{ Deleting() error }
	// DeletedHook runs after a model is deleted via DeleteModel.
	DeletedHook interface{ Deleted() error }
)

type modelEvent int

const (
	eventCreating modelEvent = iota
	eventCreated
	eventUpdating
	eventUpdated
	eventSaving
	eventSaved
	eventDeleting
	eventDeleted
)

// Observer bundles lifecycle callbacks for a model type, Laravel's model
// observers. All fields are optional.
//
//	database.Observe(database.Observer[User]{
//	    Created: func(u *User) error { return sendWelcome(u) },
//	})
type Observer[T any] struct {
	Creating func(*T) error
	Created  func(*T) error
	Updating func(*T) error
	Updated  func(*T) error
	Saving   func(*T) error
	Saved    func(*T) error
	Deleting func(*T) error
	Deleted  func(*T) error
}

var (
	observerMu       sync.RWMutex
	observerRegistry = make(map[reflect.Type]map[modelEvent][]func(any) error)
)

// Observe registers an observer for a model type. Multiple observers may
// be registered; they fire in registration order, after the model's own
// hook methods.
func Observe[T any](observer Observer[T]) {
	t := reflect.TypeOf((*T)(nil)).Elem()

	adapt := func(fn func(*T) error) func(any) error {
		return func(model any) error { return fn(model.(*T)) }
	}

	observerMu.Lock()
	defer observerMu.Unlock()
	events := observerRegistry[t]
	if events == nil {
		events = make(map[modelEvent][]func(any) error)
		observerRegistry[t] = events
	}
	register := func(event modelEvent, fn func(*T) error) {
		if fn != nil {
			events[event] = append(events[event], adapt(fn))
		}
	}
	register(eventCreating, observer.Creating)
	register(eventCreated, observer.Created)
	register(eventUpdating, observer.Updating)
	register(eventUpdated, observer.Updated)
	register(eventSaving, observer.Saving)
	register(eventSaved, observer.Saved)
	register(eventDeleting, observer.Deleting)
	register(eventDeleted, observer.Deleted)
}

// ClearObservers removes all registered observers (mainly for tests).
func ClearObservers() {
	observerMu.Lock()
	defer observerMu.Unlock()
	observerRegistry = make(map[reflect.Type]map[modelEvent][]func(any) error)
}

// fireModelEvent runs the model's own hook method (if implemented) and
// then any registered observers for the event.
func fireModelEvent[T any](model *T, event modelEvent) error {
	var hookErr error
	switch event {
	case eventCreating:
		if h, ok := any(model).(CreatingHook); ok {
			hookErr = h.Creating()
		}
	case eventCreated:
		if h, ok := any(model).(CreatedHook); ok {
			hookErr = h.Created()
		}
	case eventUpdating:
		if h, ok := any(model).(UpdatingHook); ok {
			hookErr = h.Updating()
		}
	case eventUpdated:
		if h, ok := any(model).(UpdatedHook); ok {
			hookErr = h.Updated()
		}
	case eventSaving:
		if h, ok := any(model).(SavingHook); ok {
			hookErr = h.Saving()
		}
	case eventSaved:
		if h, ok := any(model).(SavedHook); ok {
			hookErr = h.Saved()
		}
	case eventDeleting:
		if h, ok := any(model).(DeletingHook); ok {
			hookErr = h.Deleting()
		}
	case eventDeleted:
		if h, ok := any(model).(DeletedHook); ok {
			hookErr = h.Deleted()
		}
	}
	if hookErr != nil {
		return hookErr
	}

	t := reflect.TypeOf(*model)
	observerMu.RLock()
	callbacks := observerRegistry[t][event]
	observerMu.RUnlock()
	for _, callback := range callbacks {
		if err := callback(model); err != nil {
			return err
		}
	}
	return nil
}
