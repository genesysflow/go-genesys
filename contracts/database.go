// Package contracts defines interfaces for all major framework components.
package contracts

import (
	"context"
	"database/sql"
)

// DBTX is the interface that SQLC expects for database access.
// Both Connection and Transaction implement this interface.
type DBTX interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	PrepareContext(ctx context.Context, query string) (*sql.Stmt, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// DB defines the interface for database operations.
type DB interface {
	// Connection returns a connection by name.
	Connection(name ...string) Connection

	// Raw executes a raw SQL query.
	Raw(query string, bindings ...any) (*sql.Rows, error)

	// Select executes a raw select query.
	Select(query string, bindings ...any) (*sql.Rows, error)

	// Insert executes a raw insert query.
	Insert(query string, bindings ...any) (sql.Result, error)

	// Update executes a raw update query.
	Update(query string, bindings ...any) (sql.Result, error)

	// Delete executes a raw delete query.
	Delete(query string, bindings ...any) (sql.Result, error)

	// Statement executes a raw statement.
	Statement(query string, bindings ...any) (sql.Result, error)

	// Transaction executes a callback within a database transaction.
	Transaction(fn func(tx Transaction) error) error

	// BeginTransaction starts a new database transaction.
	BeginTransaction() (Transaction, error)

	// GetDefaultConnection returns the default connection name.
	GetDefaultConnection() string

	// SetDefaultConnection sets the default connection name.
	SetDefaultConnection(name string)

	// Disconnect disconnects from the given connection.
	Disconnect(name ...string) error

	// Reconnect reconnects to the given connection.
	Reconnect(name ...string) (Connection, error)

	// Close closes all database connections.
	Close() error
}

// Connection represents a database connection.
// It implements the DBTX interface for SQLC compatibility.
type Connection interface {
	DBTX

	// Name returns the connection name.
	Name() string

	// Driver returns the driver name (e.g., "pgsql", "sqlite").
	Driver() string

	// DB returns the underlying *sql.DB instance.
	// Pass this to SQLC-generated New() functions.
	DB() *sql.DB

	// Prefix returns the table prefix.
	Prefix() string

	// Query executes a raw query.
	Query(query string, bindings ...any) (*sql.Rows, error)

	// QueryContext executes a raw query with context.
	QueryContext(ctx context.Context, query string, bindings ...any) (*sql.Rows, error)

	// QueryRow executes a query that returns at most one row.
	QueryRow(query string, bindings ...any) *sql.Row

	// QueryRowContext executes a query that returns at most one row with context.
	QueryRowContext(ctx context.Context, query string, bindings ...any) *sql.Row

	// Exec executes a raw statement.
	Exec(query string, bindings ...any) (sql.Result, error)

	// ExecContext executes a raw statement with context.
	ExecContext(ctx context.Context, query string, bindings ...any) (sql.Result, error)

	// Prepare prepares a statement.
	Prepare(query string) (*sql.Stmt, error)

	// PrepareContext prepares a statement with context.
	PrepareContext(ctx context.Context, query string) (*sql.Stmt, error)

	// BeginTransaction starts a transaction.
	BeginTransaction() (Transaction, error)

	// BeginTx starts a transaction with options.
	BeginTx(ctx context.Context, opts *sql.TxOptions) (Transaction, error)

	// Transaction runs a callback in a transaction.
	Transaction(fn func(tx Transaction) error) error

	// Close closes the connection.
	Close() error

	// Ping verifies the connection is alive.
	Ping() error

	// PingContext verifies the connection is alive with context.
	PingContext(ctx context.Context) error

	// Error returns any connection error.
	Error() error
}

// Transaction represents an active database transaction.
// It implements the DBTX interface for SQLC compatibility.
type Transaction interface {
	DBTX

	// Query executes a query within the transaction.
	Query(query string, bindings ...any) (*sql.Rows, error)

	// QueryContext executes a query within the transaction with context.
	QueryContext(ctx context.Context, query string, bindings ...any) (*sql.Rows, error)

	// QueryRow executes a query that returns at most one row.
	QueryRow(query string, bindings ...any) *sql.Row

	// QueryRowContext executes a query that returns at most one row with context.
	QueryRowContext(ctx context.Context, query string, bindings ...any) *sql.Row

	// Exec executes a statement within the transaction.
	Exec(query string, bindings ...any) (sql.Result, error)

	// ExecContext executes a statement within the transaction with context.
	ExecContext(ctx context.Context, query string, bindings ...any) (sql.Result, error)

	// Prepare prepares a statement within the transaction.
	Prepare(query string) (*sql.Stmt, error)

	// PrepareContext prepares a statement within the transaction with context.
	PrepareContext(ctx context.Context, query string) (*sql.Stmt, error)

	// Tx returns the underlying *sql.Tx.
	// Pass this to SQLC-generated New() functions.
	Tx() *sql.Tx

	// Commit commits the transaction.
	Commit() error

	// Rollback rolls back the transaction.
	Rollback() error
}
