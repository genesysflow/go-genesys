package db

import (
	"github.com/genesysflow/go-genesys/query"
)

// Table starts a fluent query against the given table on the default
// connection (or a named connection when provided):
//
//	users, err := db.Table("users").Where("active", true).OrderBy("name").Get()
func Table(table string, connection ...string) *query.Builder {
	conn := Connection(connection...)
	if conn == nil {
		// Return a builder wired to a nil executor; execution will panic with
		// a clear message rather than silently doing nothing.
		panic("db: facade not initialised - call db.SetInstance during bootstrap")
	}
	return query.New(conn.Driver(), conn).Table(table)
}
