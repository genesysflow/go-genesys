package database

import (
	"time"
)

// QueryEvent describes one executed query, DB::listen's payload.
type QueryEvent struct {
	// Connection is the connection name the query ran on.
	Connection string

	// SQL is the executed statement.
	SQL string

	// Bindings are the bound parameters.
	Bindings []any

	// Duration is how long the query took.
	Duration time.Duration

	// Err is the query error, if any.
	Err error
}

// Listen registers a callback fired after every query executed through
// this manager's connections - Laravel's DB::listen. Use it for query
// logging and slow-query detection:
//
//	manager.Listen(func(e database.QueryEvent) {
//	    if e.Duration > 100*time.Millisecond {
//	        log.Warn("slow query", "sql", e.SQL, "took", e.Duration)
//	    }
//	})
func (m *Manager) Listen(fn func(QueryEvent)) {
	m.listenMu.Lock()
	defer m.listenMu.Unlock()
	m.queryListeners = append(m.queryListeners, fn)
}

// ClearListeners removes this manager's query listeners.
func (m *Manager) ClearListeners() {
	m.listenMu.Lock()
	defer m.listenMu.Unlock()
	m.queryListeners = nil
}

// fireQueryEvent notifies the manager's listeners about a query.
func (c *Connection) fireQueryEvent(sqlQuery string, bindings []any, start time.Time, err error) {
	if c.manager == nil {
		return
	}
	c.manager.listenMu.RLock()
	callbacks := c.manager.queryListeners
	c.manager.listenMu.RUnlock()
	if len(callbacks) == 0 {
		return
	}
	event := QueryEvent{
		Connection: c.name,
		SQL:        sqlQuery,
		Bindings:   bindings,
		Duration:   time.Since(start),
		Err:        err,
	}
	for _, callback := range callbacks {
		callback(event)
	}
}
