package database

import "reflect"

// FirstOrCreate returns the first model matching the attribute map, or
// inserts the given model (whose fields should include the match
// attributes) and returns it - Laravel's firstOrCreate:
//
//	user, err := database.FirstOrCreate(map[string]any{"email": "a@x.io"},
//	    &User{Email: "a@x.io", Name: "Ada"})
//
// The lookup and insert are separate statements; wrap the call in
// WithinTransaction and pass the scope when racing writers matter.
func FirstOrCreate[T any](match map[string]any, model *T, scope ...*TxScope) (*T, error) {
	q := Query[T](scope...)
	for column, value := range match {
		q.Where(column, value)
	}
	existing, err := q.First()
	if err == nil {
		return existing, nil
	}
	if err != ErrNotFound {
		return nil, err
	}
	if err := Create(model, scope...); err != nil {
		return nil, err
	}
	return model, nil
}

// UpdateOrCreate finds the first model matching the attribute map and
// updates it with the given model's fields, or inserts the model when
// nothing matches - Laravel's updateOrCreate:
//
//	user, err := database.UpdateOrCreate(map[string]any{"email": "a@x.io"},
//	    &User{Email: "a@x.io", Name: "New Name"})
func UpdateOrCreate[T any](match map[string]any, model *T, scope ...*TxScope) (*T, error) {
	q := Query[T](scope...)
	for column, value := range match {
		q.Where(column, value)
	}
	existing, err := q.First()
	if err == ErrNotFound {
		if err := Create(model, scope...); err != nil {
			return nil, err
		}
		return model, nil
	}
	if err != nil {
		return nil, err
	}

	// Carry the found row's identity onto the caller's model, keep its
	// creation time, and persist the new field values.
	meta, metaErr := metaFor(modelType(model))
	if metaErr != nil {
		return nil, metaErr
	}
	setPK(meta, model, pkValue(meta, existing))
	copyCreatedAt(meta, existing, model)
	if err := Update(model, scope...); err != nil {
		return nil, err
	}
	return model, nil
}

// modelType returns the struct type behind a model pointer.
func modelType[T any](model *T) reflect.Type {
	return reflect.TypeOf(model).Elem()
}

// copyCreatedAt keeps the original creation timestamp when a model is
// updated in place of an existing row.
func copyCreatedAt[T any](meta *modelMeta, from, to *T) {
	for _, f := range meta.fields {
		if f.isCreated {
			source := reflect.ValueOf(from).Elem().FieldByIndex(f.index)
			target := reflect.ValueOf(to).Elem().FieldByIndex(f.index)
			if target.CanSet() {
				target.Set(source)
			}
		}
	}
}
