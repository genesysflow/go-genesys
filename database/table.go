package database

import (
	"github.com/genesysflow/go-genesys/contracts"
	"github.com/genesysflow/go-genesys/query"
)

// Table starts a fluent query against the given table on the default
// connection (or a named connection when provided).
func (m *Manager) Table(table string, connection ...string) *query.Builder {
	conn := m.Connection(connection...)
	return query.New(conn.Driver(), conn).Table(table)
}

// TableOn starts a fluent query against the given table using any executor
// (connection or transaction) with the given driver's grammar.
func TableOn(driver string, executor query.Executor, table string) *query.Builder {
	return query.New(driver, executor).Table(table)
}

// TableTx starts a fluent query bound to a transaction, using the driver of
// the given connection for SQL grammar.
func TableTx(conn contracts.Connection, tx contracts.Transaction, table string) *query.Builder {
	return query.New(conn.Driver(), tx).Table(table)
}
