package database

import (
	"fmt"
	"reflect"
	"time"

	"github.com/genesysflow/go-genesys/query"
)

// Increment atomically adds amount (default 1) to a column on every
// matching row:
//
//	database.Query[Post]().Where("id", id).Increment("views")
func (q *ModelQuery[T]) Increment(column string, amount ...int) (int64, error) {
	if q.err != nil {
		return 0, q.err
	}
	q.applySoftDeleteScope()
	return q.builder.Increment(column, amount...)
}

// Decrement atomically subtracts amount (default 1) from a column.
func (q *ModelQuery[T]) Decrement(column string, amount ...int) (int64, error) {
	if q.err != nil {
		return 0, q.err
	}
	q.applySoftDeleteScope()
	return q.builder.Decrement(column, amount...)
}

// Upsert bulk-inserts rows for T's table, updating rows that collide on
// the uniqueBy columns - Laravel's Model::upsert(). With update nil,
// every non-unique column is updated. The uniqueBy columns must be
// covered by a unique index:
//
//	database.Upsert[Flight]([]map[string]any{
//	    {"code": "BA9", "price": 240},
//	    {"code": "AF3", "price": 180},
//	}, []string{"code"}, []string{"price"})
func Upsert[T any](rows []map[string]any, uniqueBy []string, update []string, scope ...*TxScope) (int64, error) {
	meta, err := metaFor(reflect.TypeOf((*T)(nil)).Elem())
	if err != nil {
		return 0, err
	}
	driver, executor := executorFor[T](scope)

	// Stamp timestamps the way Create does, without overriding caller
	// values.
	now := time.Now().UTC().Truncate(time.Second)
	_, hasCreated := meta.byCol["created_at"]
	_, hasUpdated := meta.byCol["updated_at"]
	stamped := make([]map[string]any, len(rows))
	for i, row := range rows {
		copied := make(map[string]any, len(row)+2)
		for k, v := range row {
			copied[k] = v
		}
		if hasCreated {
			if _, ok := copied["created_at"]; !ok {
				copied["created_at"] = now
			}
		}
		if hasUpdated {
			if _, ok := copied["updated_at"]; !ok {
				copied["updated_at"] = now
			}
		}
		stamped[i] = copied
	}

	return query.New(driver, executor).Table(meta.table).Upsert(stamped, uniqueBy, update)
}

// Touch bumps the model's updated_at to now, in the database and on the
// struct - Laravel's $model->touch().
func Touch[T any](model *T, scope ...*TxScope) error {
	meta, err := metaFor(reflect.TypeOf(model).Elem())
	if err != nil {
		return err
	}
	updatedField, ok := meta.byCol["updated_at"]
	if !ok {
		return fmt.Errorf("database: %s has no updated_at column to touch", meta.table)
	}
	id := pkValue(meta, model)
	if id == 0 {
		return fmt.Errorf("database: cannot touch %s without an id", meta.table)
	}

	now := time.Now().UTC().Truncate(time.Second)
	driver, executor := executorFor[T](scope)
	affected, err := query.New(driver, executor).Table(meta.table).
		Where("id", id).
		Update(map[string]any{"updated_at": now})
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrNotFound
	}

	reflect.ValueOf(model).Elem().FieldByIndex(updatedField.index).Set(reflect.ValueOf(now))
	if original := GetOriginal(model); original != nil {
		original["updated_at"] = now
	}
	return nil
}
