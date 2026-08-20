package database

import (
	"fmt"
	"reflect"

	"github.com/genesysflow/go-genesys/query"
)

// ModelCursorPage is one typed page of cursor-paginated models.
type ModelCursorPage[T any] struct {
	Data       []T    `json:"data"`
	PerPage    int    `json:"per_page"`
	NextCursor string `json:"next_cursor,omitempty"`
}

// CursorPaginate pages forward through models ordered by a unique
// column (default "id"); pass the previous page's NextCursor to
// continue. Unlike offset pagination it never rescans skipped rows and
// stays stable under concurrent inserts:
//
//	page, _ := database.Query[User]().Where("active", true).CursorPaginate(25, cursor)
//	next := page.NextCursor // "" on the last page
func (q *ModelQuery[T]) CursorPaginate(perPage int, cursor string, column ...string) (*ModelCursorPage[T], error) {
	if q.err != nil {
		return nil, q.err
	}
	if perPage < 1 {
		perPage = 15
	}
	col := "id"
	if len(column) > 0 && column[0] != "" {
		col = column[0]
	}
	colField, ok := q.meta.byCol[col]
	if !ok {
		return nil, fmt.Errorf("database: cursor column %q is not a column of %s", col, q.meta.table)
	}

	q.applySoftDeleteScope()
	builder := q.builder.Clone().OrderBy(col).Limit(perPage + 1)
	if cursor != "" {
		after, err := query.DecodeCursor(cursor)
		if err != nil {
			return nil, err
		}
		builder.Where(col, ">", after)
	}

	rows, err := builder.Rows()
	if err != nil {
		return nil, err
	}
	items, err := scanRowsInto[T](rows)
	if err != nil {
		return nil, err
	}

	page := &ModelCursorPage[T]{PerPage: perPage}
	if len(items) > perPage {
		items = items[:perPage]
		lastValue := reflect.ValueOf(items[len(items)-1]).FieldByIndex(colField.index).Interface()
		page.NextCursor = query.EncodeCursor(lastValue)
	}
	if err := q.loadWiths(items); err != nil {
		return nil, err
	}
	page.Data = items
	return page, nil
}

// Chunk processes matching models in primary-key-ordered batches,
// Laravel's chunkById. Each batch is fetched with a fresh keyset query
// (id > last seen), so rows deleted or updated inside fn cannot shift
// the window. Returning an error from fn stops the iteration.
//
//	err := database.Query[User]().Where("active", true).Chunk(500, func(users []User) error {
//	    return exportBatch(users)
//	})
func (q *ModelQuery[T]) Chunk(size int, fn func(items []T) error) error {
	if q.err != nil {
		return q.err
	}
	if size < 1 {
		return fmt.Errorf("database: chunk size must be positive")
	}
	if q.meta.pkIndex < 0 {
		return fmt.Errorf("database: %s has no id column to chunk by", q.meta.table)
	}
	pkField := q.meta.fields[q.meta.pkIndex]

	q.applySoftDeleteScope()
	var last any
	for {
		builder := q.builder.Clone().OrderBy(pkField.column).Limit(size)
		if last != nil {
			builder.Where(pkField.column, ">", last)
		}
		rows, err := builder.Rows()
		if err != nil {
			return err
		}
		items, err := scanRowsInto[T](rows)
		if err != nil {
			return err
		}
		if len(items) == 0 {
			return nil
		}
		if err := q.loadWiths(items); err != nil {
			return err
		}
		if err := fn(items); err != nil {
			return err
		}
		last = reflect.ValueOf(items[len(items)-1]).FieldByIndex(pkField.index).Interface()
		if len(items) < size {
			return nil
		}
	}
}

// Each runs fn for every matching model, fetching in chunks of 100
// under the hood. Returning an error stops the iteration.
func (q *ModelQuery[T]) Each(fn func(item *T) error) error {
	return q.Chunk(100, func(items []T) error {
		for i := range items {
			if err := fn(&items[i]); err != nil {
				return err
			}
		}
		return nil
	})
}
