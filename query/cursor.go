package query

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
)

// CursorPage is one page of cursor-paginated rows. Cursor pagination
// scales to arbitrary depth (no OFFSET scan) and stays stable while
// rows are inserted, at the cost of only paging forward.
type CursorPage struct {
	Data       []map[string]any `json:"data"`
	PerPage    int              `json:"per_page"`
	NextCursor string           `json:"next_cursor,omitempty"`
}

// EncodeCursor encodes a cursor value for the client.
func EncodeCursor(value any) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(encoded)
}

// DecodeCursor decodes a client-supplied cursor.
func DecodeCursor(cursor string) (any, error) {
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return nil, fmt.Errorf("query: malformed cursor: %w", err)
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, fmt.Errorf("query: malformed cursor: %w", err)
	}
	return value, nil
}

// CursorPaginate pages forward through rows ordered by a unique column
// (default "id"). Pass the previous page's NextCursor to continue; an
// empty NextCursor means the last page:
//
//	page, _ := db.Table("users").Where("active", true).CursorPaginate(25, cursor)
func (b *Builder) CursorPaginate(perPage int, cursor string, column ...string) (*CursorPage, error) {
	if perPage < 1 {
		perPage = 15
	}
	col := "id"
	if len(column) > 0 && column[0] != "" {
		col = column[0]
	}

	builder := b.Clone().OrderBy(col).Limit(perPage + 1)
	if cursor != "" {
		after, err := DecodeCursor(cursor)
		if err != nil {
			return nil, err
		}
		builder.Where(col, ">", after)
	}

	rows, err := builder.Get()
	if err != nil {
		return nil, err
	}

	page := &CursorPage{PerPage: perPage}
	if len(rows) > perPage {
		rows = rows[:perPage]
		page.NextCursor = EncodeCursor(rows[len(rows)-1][col])
	}
	page.Data = rows
	return page, nil
}
