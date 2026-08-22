package database

import (
	"fmt"
	"reflect"
	"slices"
)

// Fresh re-reads the model from the database and returns the stored row
// as a new instance, leaving the one in hand untouched:
//
//	stored, err := database.Fresh(user)
func Fresh[T any](model *T, scope ...*TxScope) (*T, error) {
	if model == nil {
		return nil, fmt.Errorf("database: cannot refresh a nil model")
	}

	meta, err := metaFor(reflect.TypeOf(model))
	if err != nil {
		return nil, err
	}

	id := pkValue(meta, model)
	if id == 0 {
		return nil, fmt.Errorf("database: cannot refresh a model with no primary key")
	}

	fresh, err := Find[T](id, scope...)
	if err != nil {
		return nil, err
	}
	if fresh == nil {
		return nil, fmt.Errorf("database: row %d no longer exists", id)
	}
	return fresh, nil
}

// Refresh re-reads the model from the database in place, discarding
// unsaved changes. A row that has since been deleted is an error rather
// than a silently stale model.
func Refresh[T any](model *T, scope ...*TxScope) error {
	fresh, err := Fresh(model, scope...)
	if err != nil {
		return err
	}

	*model = *fresh
	return nil
}

// Replicate copies a model as an unsaved new record: the primary key and
// timestamps are cleared, so Create inserts a new row. Columns named in
// except are left at their zero value.
//
//	copy := database.Replicate(order, "invoice_number")
func Replicate[T any](model *T, except ...string) *T {
	replica := new(T)
	if model == nil {
		return replica
	}
	*replica = *model

	meta, err := metaFor(reflect.TypeOf(model))
	if err != nil {
		return replica
	}

	value := reflect.ValueOf(replica).Elem()
	for i := range meta.fields {
		field := &meta.fields[i]

		// A replica is a new row: it has no identity and no history.
		if field.isPK || field.isCreated || field.isUpdated || field.isDeleted ||
			slices.Contains(except, field.column) {
			target := value.FieldByIndex(field.index)
			target.Set(reflect.Zero(target.Type()))
		}
	}

	// The replica has never been read from the database, so it has no
	// pristine copy to diff against.
	if holder, ok := any(replica).(originalHolder); ok {
		holder.setOriginal(nil)
	}

	return replica
}

// Is reports whether two models are the same record: the same type with
// the same primary key. A nil model is never the same as anything.
func Is[T any](first, second *T) bool {
	if first == nil || second == nil {
		return false
	}

	meta, err := metaFor(reflect.TypeOf(first))
	if err != nil {
		return false
	}

	id := pkValue(meta, first)
	return id != 0 && id == pkValue(meta, second)
}

// HasHidden is implemented by models that keep columns out of their
// serialized form - a password hash has no business in a response.
type HasHidden interface {
	Hidden() []string
}

// HasVisible is the allow-list form: only the named columns are
// serialized.
type HasVisible interface {
	Visible() []string
}

// HasAppends is implemented by models that add computed values to their
// serialized form, Laravel's $appends.
type HasAppends interface {
	Appends() map[string]any
}

// ToMap renders a model as the map that should be serialized, honouring
// Hidden, Visible and Appends:
//
//	return ctx.Resource(database.ToMap(user))
//
// Encoding a model directly with encoding/json cannot do this - struct
// tags are fixed at compile time - so a model with anything to hide is
// rendered through here.
func ToMap[T any](model *T) map[string]any {
	if model == nil {
		return nil
	}

	meta, err := metaFor(reflect.TypeOf(model))
	if err != nil {
		return nil
	}

	var hidden, visible []string
	if hider, ok := any(model).(HasHidden); ok {
		hidden = hider.Hidden()
	}
	if shower, ok := any(model).(HasVisible); ok {
		visible = shower.Visible()
	}

	value := reflect.ValueOf(model).Elem()
	rendered := make(map[string]any, len(meta.fields))

	for i := range meta.fields {
		field := &meta.fields[i]

		if len(visible) > 0 && !slices.Contains(visible, field.column) {
			continue
		}
		if slices.Contains(hidden, field.column) {
			continue
		}

		rendered[field.column] = value.FieldByIndex(field.index).Interface()
	}

	if appender, ok := any(model).(HasAppends); ok {
		for key, appended := range appender.Appends() {
			rendered[key] = appended
		}
	}

	return rendered
}

// ToMapSlice renders many models, for a collection response.
func ToMapSlice[T any](models []T) []map[string]any {
	rendered := make([]map[string]any, 0, len(models))
	for i := range models {
		rendered = append(rendered, ToMap(&models[i]))
	}
	return rendered
}
