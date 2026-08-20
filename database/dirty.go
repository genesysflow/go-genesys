package database

import (
	"reflect"
	"time"
)

// SyncOriginal snapshots the model's current column values as its
// pristine state. The ORM calls this after fetching, creating, and
// updating; call it manually only when bypassing the ORM.
func SyncOriginal[T any](model *T) {
	meta, err := metaFor(reflect.TypeOf(model).Elem())
	if err != nil {
		return
	}
	if holder, ok := any(model).(originalHolder); ok {
		holder.setOriginal(meta.values(reflect.ValueOf(model).Elem(), false))
	}
}

// GetOriginal returns the column values as last synced with the
// database, or nil for a model that was never fetched or saved.
func GetOriginal[T any](model *T) map[string]any {
	if holder, ok := any(model).(originalHolder); ok {
		return holder.getOriginal()
	}
	return nil
}

// GetDirty returns the columns whose current value differs from the last
// synced state, mapped to their new values. A model with no synced state
// reports every column as dirty.
func GetDirty[T any](model *T) map[string]any {
	meta, err := metaFor(reflect.TypeOf(model).Elem())
	if err != nil {
		return nil
	}
	current := meta.values(reflect.ValueOf(model).Elem(), false)

	original := GetOriginal(model)
	if original == nil {
		return current
	}

	dirty := make(map[string]any)
	for column, value := range current {
		if !columnValuesEqual(original[column], value) {
			dirty[column] = value
		}
	}
	return dirty
}

// IsDirty reports whether any column (or one of the given columns)
// changed since the model was last synced with the database.
func IsDirty[T any](model *T, columns ...string) bool {
	dirty := GetDirty(model)
	if len(columns) == 0 {
		return len(dirty) > 0
	}
	for _, column := range columns {
		if _, ok := dirty[column]; ok {
			return true
		}
	}
	return false
}

// columnValuesEqual compares two column values, treating time.Time by
// instant (a wall-clock reading with and without monotonic time still
// compares equal).
func columnValuesEqual(a, b any) bool {
	if at, ok := a.(time.Time); ok {
		if bt, ok := b.(time.Time); ok {
			return at.Equal(bt)
		}
	}
	if ap, ok := a.(*time.Time); ok {
		if bp, ok := b.(*time.Time); ok {
			if ap == nil || bp == nil {
				return ap == bp
			}
			return ap.Equal(*bp)
		}
	}
	return reflect.DeepEqual(a, b)
}
