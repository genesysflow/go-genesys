package database

import (
	"reflect"
	"time"
)

// WithTrashed includes soft-deleted rows in the query results.
func (q *ModelQuery[T]) WithTrashed() *ModelQuery[T] {
	q.trashed = trashedInclude
	return q
}

// OnlyTrashed restricts the query to soft-deleted rows.
func (q *ModelQuery[T]) OnlyTrashed() *ModelQuery[T] {
	q.trashed = trashedOnly
	return q
}

// Restore un-trashes all matching soft-deleted rows and returns how many
// were restored. Trashed rows are included automatically.
func (q *ModelQuery[T]) Restore() (int64, error) {
	if q.err != nil {
		return 0, q.err
	}
	if q.meta == nil || !q.meta.softDeletes {
		return 0, nil
	}
	if q.trashed == trashedExclude {
		q.trashed = trashedInclude
	}
	q.applySoftDeleteScope()
	values := map[string]any{"deleted_at": nil}
	if _, ok := q.meta.byCol["updated_at"]; ok {
		values["updated_at"] = time.Now().UTC().Truncate(time.Second)
	}
	return q.builder.Update(values)
}

// ForceDelete permanently removes all matching rows, trashed or not.
func (q *ModelQuery[T]) ForceDelete() (int64, error) {
	if q.err != nil {
		return 0, q.err
	}
	q.trashed = trashedInclude
	q.applySoftDeleteScope()
	return q.builder.Delete()
}

// Scope applies reusable query fragments, Laravel's local scopes:
//
//	func Active(q *database.ModelQuery[User]) { q.Where("active", true) }
//	users, _ := database.Query[User]().Scope(Active).Get()
func (q *ModelQuery[T]) Scope(scopes ...func(*ModelQuery[T])) *ModelQuery[T] {
	for _, scope := range scopes {
		scope(q)
	}
	return q
}

// Restore un-trashes the row with the given primary key.
func Restore[T any](id any, scope ...*TxScope) error {
	affected, err := Query[T](scope...).Where("id", id).Restore()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

// ForceDelete permanently removes the row with the given primary key,
// even for soft-deletable models.
func ForceDelete[T any](id any, scope ...*TxScope) error {
	affected, err := Query[T](scope...).Where("id", id).ForceDelete()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

// markDeletedAt mirrors a soft delete onto the in-memory model so its
// DeletedAt field matches the database row.
func markDeletedAt[T any](meta *modelMeta, model *T, at *time.Time) {
	field, ok := meta.byCol["deleted_at"]
	if !ok {
		return
	}
	v := reflect.ValueOf(model).Elem().FieldByIndex(field.index)
	if v.Kind() != reflect.Pointer {
		return
	}
	if at == nil {
		v.Set(reflect.Zero(v.Type()))
		return
	}
	ptr := reflect.New(v.Type().Elem())
	ptr.Elem().Set(reflect.ValueOf(*at))
	v.Set(ptr)
}
