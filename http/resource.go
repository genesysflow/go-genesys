package http

import (
	"encoding/json"
	"fmt"
)

// Resource sends data wrapped in a Laravel-style "data" envelope:
//
//	{"data": {...}}
func (c *Context) Resource(data any) error {
	return c.JSONResponse(map[string]any{"data": data})
}

// ResourceWith sends data with additional top-level metadata:
//
//	{"data": {...}, "meta": {...}}
func (c *Context) ResourceWith(data any, meta map[string]any) error {
	return c.JSONResponse(map[string]any{"data": data, "meta": meta})
}

// Paginated sends a paginator (query.Paginator, database.ModelPaginator, or
// any struct with a "data" JSON field) in Laravel's paginated shape:
//
//	{"data": [...], "meta": {"total": ..., "per_page": ..., ...}}
func (c *Context) Paginated(paginator any) error {
	raw, err := json.Marshal(paginator)
	if err != nil {
		return fmt.Errorf("http: cannot serialize paginator: %w", err)
	}
	var flat map[string]json.RawMessage
	if err := json.Unmarshal(raw, &flat); err != nil {
		return fmt.Errorf("http: paginator must serialize to an object: %w", err)
	}

	data, ok := flat["data"]
	if !ok {
		return fmt.Errorf("http: paginator has no data field")
	}
	delete(flat, "data")

	meta := make(map[string]any, len(flat))
	for k, v := range flat {
		var value any
		if err := json.Unmarshal(v, &value); err != nil {
			return err
		}
		meta[k] = value
	}

	return c.JSONResponse(map[string]any{
		"data": json.RawMessage(data),
		"meta": meta,
	})
}
