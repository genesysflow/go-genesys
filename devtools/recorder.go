// Package devtools records what an application did while handling a
// request - the request itself, and the queries it ran - and serves it
// as a panel for local debugging.
//
// It is a development tool. The panel shows request paths and SQL, which
// is exactly what must not be served from a production box, so Register
// refuses to mount it there.
package devtools

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/genesysflow/go-genesys/database"
	"github.com/genesysflow/go-genesys/http"
)

// RequestEntry is one handled request.
type RequestEntry struct {
	Method    string
	Path      string
	Status    int
	Duration  time.Duration
	StartedAt time.Time
}

// QueryEntry is one executed query.
type QueryEntry struct {
	Connection   string
	SQL          string
	BindingCount int
	Bindings     []string // empty unless Recorder.ShowBindings is set
	Duration     time.Duration
	Err          string
	At           time.Time
}

// Recorder keeps the most recent requests and queries in memory. It is a
// ring buffer: a dev server left running for a day must not grow without
// bound.
type Recorder struct {
	// ShowBindings includes query binding values in what is recorded.
	// Off by default: bindings routinely carry personal data, and the
	// SQL text alone is usually what is being debugged.
	ShowBindings bool

	capacity  int
	mu        sync.RWMutex
	requests  []RequestEntry
	queries   []QueryEntry
	panelPath string
}

// NewRecorder creates a recorder holding at most capacity entries of
// each kind.
func NewRecorder(capacity int) *Recorder {
	if capacity <= 0 {
		capacity = 100
	}
	return &Recorder{capacity: capacity}
}

// RecordRequest stores a handled request.
func (r *Recorder) RecordRequest(entry RequestEntry) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.requests = append(r.requests, entry)
	if len(r.requests) > r.capacity {
		r.requests = r.requests[len(r.requests)-r.capacity:]
	}
}

// RecordQuery stores an executed query. It has the shape the database
// manager's listener wants:
//
//	manager.Listen(recorder.RecordQuery)
func (r *Recorder) RecordQuery(event database.QueryEvent) {
	entry := QueryEntry{
		Connection:   event.Connection,
		SQL:          event.SQL,
		BindingCount: len(event.Bindings),
		Duration:     event.Duration,
		At:           time.Now(),
	}
	if event.Err != nil {
		entry.Err = event.Err.Error()
	}
	if r.ShowBindings {
		entry.Bindings = make([]string, 0, len(event.Bindings))
		for _, binding := range event.Bindings {
			entry.Bindings = append(entry.Bindings, fmt.Sprint(binding))
		}
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.queries = append(r.queries, entry)
	if len(r.queries) > r.capacity {
		r.queries = r.queries[len(r.queries)-r.capacity:]
	}
}

// Requests returns the recorded requests, oldest first.
func (r *Recorder) Requests() []RequestEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]RequestEntry(nil), r.requests...)
}

// Queries returns the recorded queries, oldest first.
func (r *Recorder) Queries() []QueryEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]QueryEntry(nil), r.queries...)
}

// QueriesDuring returns the queries recorded while a request was being
// handled.
//
// The database listener carries no request identity, so this is a time
// window rather than a true correlation: under concurrent traffic it can
// include another request's queries. In local development, where the
// panel is used, that is rarely a distinction that matters.
func (r *Recorder) QueriesDuring(entry RequestEntry) []QueryEntry {
	start := entry.StartedAt
	end := start.Add(entry.Duration)

	r.mu.RLock()
	defer r.mu.RUnlock()

	during := make([]QueryEntry, 0)
	for _, query := range r.queries {
		if query.At.Before(start) || query.At.After(end) {
			continue
		}
		during = append(during, query)
	}
	return during
}

// Clear forgets everything recorded.
func (r *Recorder) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.requests = nil
	r.queries = nil
}

// setPanelPath records where the panel is mounted, so its own visits
// are not recorded.
func (r *Recorder) setPanelPath(path string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.panelPath = path
}

// isPanelPath reports whether a path belongs to the panel.
func (r *Recorder) isPanelPath(path string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.panelPath != "" && strings.HasPrefix(path, r.panelPath)
}

// Middleware records every request that passes through it.
func Middleware(recorder *Recorder) http.MiddlewareFunc {
	return func(ctx *http.Context, next func() error) error {
		started := time.Now()

		err := next()

		// The panel's own visits would otherwise fill the buffer with
		// itself.
		if recorder.isPanelPath(ctx.Path()) {
			return err
		}

		recorder.RecordRequest(RequestEntry{
			// Cloned: fiber hands back strings backed by the request
			// buffer, which is reused for the next request - a recorded
			// path would otherwise mutate under the recorder.
			Method:    strings.Clone(ctx.Method()),
			Path:      strings.Clone(ctx.Path()),
			Status:    ctx.FiberCtx().Response().StatusCode(),
			Duration:  time.Since(started),
			StartedAt: started,
		})

		return err
	}
}
