// Package db provides a static facade for database operations.
// This allows Laravel-style static access: DB.Table("users").Get()
package db

import (
	"database/sql"
	"sync"

	"github.com/genesysflow/go-genesys/contracts"
)

var (
	instance contracts.DB
	mu       sync.RWMutex
)

// SetInstance sets the database manager instance.
// This should be called during application bootstrap.
func SetInstance(db contracts.DB) {
	mu.Lock()
	defer mu.Unlock()
	instance = db
}

// GetInstance returns the database manager instance.
func GetInstance() contracts.DB {
	mu.RLock()
	defer mu.RUnlock()
	return instance
}

// Connection returns a connection by name.
func Connection(name ...string) contracts.Connection {
	mu.RLock()
	defer mu.RUnlock()
	if instance == nil {
		return nil
	}
	return instance.Connection(name...)
}

// Table starts a new query builder for the given table.
func Table(table string) contracts.QueryBuilder {
	mu.RLock()
	defer mu.RUnlock()
	if instance == nil {
		return nil
	}
	return instance.Table(table)
}

// Raw executes a raw SQL query.
func Raw(query string, bindings ...any) (*sql.Rows, error) {
	mu.RLock()
	defer mu.RUnlock()
	if instance == nil {
		return nil, ErrNoInstance
	}
	return instance.Raw(query, bindings...)
}

// Select executes a raw select query.
func Select(query string, bindings ...any) (*sql.Rows, error) {
	mu.RLock()
	defer mu.RUnlock()
	if instance == nil {
		return nil, ErrNoInstance
	}
	return instance.Select(query, bindings...)
}

// Insert executes a raw insert query.
func Insert(query string, bindings ...any) (sql.Result, error) {
	mu.RLock()
	defer mu.RUnlock()
	if instance == nil {
		return nil, ErrNoInstance
	}
	return instance.Insert(query, bindings...)
}

// Update executes a raw update query.
func Update(query string, bindings ...any) (sql.Result, error) {
	mu.RLock()
	defer mu.RUnlock()
	if instance == nil {
		return nil, ErrNoInstance
	}
	return instance.Update(query, bindings...)
}

// Delete executes a raw delete query.
func Delete(query string, bindings ...any) (sql.Result, error) {
	mu.RLock()
	defer mu.RUnlock()
	if instance == nil {
		return nil, ErrNoInstance
	}
	return instance.Delete(query, bindings...)
}

// Statement executes a raw statement.
func Statement(query string, bindings ...any) (sql.Result, error) {
	mu.RLock()
	defer mu.RUnlock()
	if instance == nil {
		return nil, ErrNoInstance
	}
	return instance.Statement(query, bindings...)
}

// Transaction executes a callback within a database transaction.
func Transaction(fn func(tx contracts.Transaction) error) error {
	mu.RLock()
	defer mu.RUnlock()
	if instance == nil {
		return ErrNoInstance
	}
	return instance.Transaction(fn)
}

// BeginTransaction starts a new database transaction.
func BeginTransaction() (contracts.Transaction, error) {
	mu.RLock()
	defer mu.RUnlock()
	if instance == nil {
		return nil, ErrNoInstance
	}
	return instance.BeginTransaction()
}

// GetDefaultConnection returns the default connection name.
func GetDefaultConnection() string {
	mu.RLock()
	defer mu.RUnlock()
	if instance == nil {
		return ""
	}
	return instance.GetDefaultConnection()
}

// SetDefaultConnection sets the default connection name.
func SetDefaultConnection(name string) {
	mu.RLock()
	defer mu.RUnlock()
	if instance == nil {
		return
	}
	instance.SetDefaultConnection(name)
}

// Disconnect disconnects from the given connection.
func Disconnect(name ...string) error {
	mu.RLock()
	defer mu.RUnlock()
	if instance == nil {
		return ErrNoInstance
	}
	return instance.Disconnect(name...)
}

// Reconnect reconnects to the given connection.
func Reconnect(name ...string) (contracts.Connection, error) {
	mu.RLock()
	defer mu.RUnlock()
	if instance == nil {
		return nil, ErrNoInstance
	}
	return instance.Reconnect(name...)
}

// ErrNoInstance is returned when the database facade is not initialized.
var ErrNoInstance = &NoInstanceError{}

// NoInstanceError indicates the facade has not been initialized.
type NoInstanceError struct{}

func (e *NoInstanceError) Error() string {
	return "database facade not initialized: call db.SetInstance() first"
}
