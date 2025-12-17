// Package contracts defines interfaces for all major framework components.
package contracts

import (
	"context"
	"database/sql"
)

// DB defines the interface for database operations.
// It follows Laravel's Database Manager pattern.
type DB interface {
	// Connection returns a connection by name.
	Connection(name ...string) Connection

	// Table starts a new query builder for the given table.
	Table(table string) QueryBuilder

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
}

// Connection represents a database connection.
type Connection interface {
	// Name returns the connection name.
	Name() string

	// Driver returns the driver name (e.g., "pgsql", "sqlite").
	Driver() string

	// DB returns the underlying *sql.DB instance.
	DB() *sql.DB

	// Table starts a query builder for the given table.
	Table(table string) QueryBuilder

	// Query executes a raw query.
	Query(query string, bindings ...any) (*sql.Rows, error)

	// Exec executes a raw statement.
	Exec(query string, bindings ...any) (sql.Result, error)

	// Prepare prepares a statement.
	Prepare(query string) (*sql.Stmt, error)

	// BeginTransaction starts a transaction.
	BeginTransaction() (Transaction, error)

	// Transaction runs a callback in a transaction.
	Transaction(fn func(tx Transaction) error) error

	// Close closes the connection.
	Close() error

	// Ping verifies the connection is alive.
	Ping() error

	// PingContext verifies the connection is alive with context.
	PingContext(ctx context.Context) error

	// QueryContext executes a raw query with context.
	QueryContext(ctx context.Context, query string, bindings ...any) (*sql.Rows, error)

	// ExecContext executes a raw statement with context.
	ExecContext(ctx context.Context, query string, bindings ...any) (sql.Result, error)
}

// Transaction represents an active database transaction.
type Transaction interface {
	// Query executes a query within the transaction.
	Query(query string, bindings ...any) (*sql.Rows, error)

	// QueryContext executes a query within the transaction with context.
	QueryContext(ctx context.Context, query string, bindings ...any) (*sql.Rows, error)

	// Exec executes a statement within the transaction.
	Exec(query string, bindings ...any) (sql.Result, error)

	// ExecContext executes a statement within the transaction with context.
	ExecContext(ctx context.Context, query string, bindings ...any) (sql.Result, error)

	// Table starts a query builder within the transaction.
	Table(table string) QueryBuilder

	// Commit commits the transaction.
	Commit() error

	// Rollback rolls back the transaction.
	Rollback() error
}

// QueryBuilder defines the fluent query builder interface.
type QueryBuilder interface {
	// WithContext sets the context for the query.
	WithContext(ctx context.Context) QueryBuilder

	// Select specifies the columns to select.
	Select(columns ...string) QueryBuilder

	// SelectRaw adds a raw select expression.
	SelectRaw(expression string, bindings ...any) QueryBuilder

	// Distinct constrains the query to return distinct results.
	Distinct() QueryBuilder

	// From sets the table to query from.
	From(table string) QueryBuilder

	// Join adds a join clause.
	Join(table, first, operator, second string) QueryBuilder

	// LeftJoin adds a left join clause.
	LeftJoin(table, first, operator, second string) QueryBuilder

	// RightJoin adds a right join clause.
	RightJoin(table, first, operator, second string) QueryBuilder

	// CrossJoin adds a cross join clause.
	CrossJoin(table string) QueryBuilder

	// Where adds a where clause.
	Where(column string, operator string, value any) QueryBuilder

	// OrWhere adds an or where clause.
	OrWhere(column string, operator string, value any) QueryBuilder

	// WhereIn adds a where in clause.
	WhereIn(column string, values []any) QueryBuilder

	// WhereNotIn adds a where not in clause.
	WhereNotIn(column string, values []any) QueryBuilder

	// WhereNull adds a where null clause.
	WhereNull(column string) QueryBuilder

	// WhereNotNull adds a where not null clause.
	WhereNotNull(column string) QueryBuilder

	// WhereBetween adds a where between clause.
	WhereBetween(column string, low, high any) QueryBuilder

	// WhereRaw adds a raw where clause.
	WhereRaw(sql string, bindings ...any) QueryBuilder

	// GroupBy adds a group by clause.
	GroupBy(columns ...string) QueryBuilder

	// Having adds a having clause.
	Having(column, operator string, value any) QueryBuilder

	// HavingRaw adds a raw having clause.
	HavingRaw(sql string, bindings ...any) QueryBuilder

	// OrderBy adds an order by clause.
	OrderBy(column, direction string) QueryBuilder

	// OrderByDesc adds descending order by clause.
	OrderByDesc(column string) QueryBuilder

	// OrderByRaw adds a raw order by clause.
	OrderByRaw(sql string, bindings ...any) QueryBuilder

	// Limit limits the number of results.
	Limit(limit int) QueryBuilder

	// Offset sets the offset for the query.
	Offset(offset int) QueryBuilder

	// Take is an alias for Limit.
	Take(count int) QueryBuilder

	// Skip is an alias for Offset.
	Skip(count int) QueryBuilder

	// ForPage paginates the results.
	ForPage(page, perPage int) QueryBuilder

	// Get executes the query and returns all results.
	Get(dest ...any) ([]map[string]any, error)

	// First returns the first result.
	First(dest ...any) (map[string]any, error)

	// Find finds a record by primary key.
	Find(id any, dest ...any) (map[string]any, error)

	// Value returns a single column value.
	Value(column string) (any, error)

	// Pluck returns a slice of values for a column.
	Pluck(column string) ([]any, error)

	// Exists checks if any records exist.
	Exists() (bool, error)

	// DoesntExist checks if no records exist.
	DoesntExist() (bool, error)

	// Count returns the count of records.
	Count() (int64, error)

	// Max returns the max value of a column.
	Max(column string) (any, error)

	// Min returns the min value of a column.
	Min(column string) (any, error)

	// Sum returns the sum of a column.
	Sum(column string) (float64, error)

	// Avg returns the average of a column.
	Avg(column string) (float64, error)

	// Insert inserts a record.
	Insert(values map[string]any) (int64, error)

	// InsertGetId inserts a record and returns the inserted ID.
	InsertGetId(values map[string]any) (int64, error)

	// InsertBatch inserts multiple records.
	InsertBatch(records []map[string]any) (int64, error)

	// Update updates records.
	Update(values map[string]any) (int64, error)

	// Increment increments a column.
	Increment(column string, amount ...int) (int64, error)

	// Decrement decrements a column.
	Decrement(column string, amount ...int) (int64, error)

	// Delete deletes records.
	Delete() (int64, error)

	// Truncate truncates the table.
	Truncate() error

	// ToSQL returns the generated SQL.
	ToSQL() (string, []any)

	// Clone clones the query builder.
	Clone() QueryBuilder
}

// Grammar compiles query builder components into SQL.
type Grammar interface {
	// CompileSelect compiles a select query.
	CompileSelect(builder QueryBuilder) (string, []any)

	// CompileInsert compiles an insert query.
	CompileInsert(builder QueryBuilder, values map[string]any) (string, []any)

	// CompileUpdate compiles an update query.
	CompileUpdate(builder QueryBuilder, values map[string]any) (string, []any)

	// CompileDelete compiles a delete query.
	CompileDelete(builder QueryBuilder) (string, []any)

	// CompileExists compiles an exists query.
	CompileExists(builder QueryBuilder) (string, []any)

	// GetDateFormat returns the database date format.
	GetDateFormat() string

	// WrapTable wraps a table name with quotes.
	WrapTable(table string) string

	// WrapColumn wraps a column name with quotes.
	WrapColumn(column string) string

	// Parameter returns the placeholder for a binding.
	Parameter(index int) string
}
