// Package db provides a static facade for database operations.
// For SQLC usage: pass db.Connection().DB() to your generated New() function.
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
// Pass Connection().DB() to SQLC-generated New() functions.
func Connection(name ...string) contracts.Connection {
	mu.RLock()
	defer mu.RUnlock()
	if instance == nil {
		return nil
	}
	return instance.Connection(name...)
}

// DB returns the underlying *sql.DB for the default connection.
// Shorthand for Connection().DB() - use with SQLC.
func DB(name ...string) *sql.DB {
	conn := Connection(name...)
	if conn == nil {
		return nil
	}
	return conn.DB()
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
// The transaction implements contracts.DBTX for SQLC compatibility.
func Transaction(fn func(tx contracts.Transaction) error) error {
	mu.RLock()
	defer mu.RUnlock()
	if instance == nil {
		return ErrNoInstance
	}
	return instance.Transaction(fn)
}

// BeginTransaction starts a new database transaction.
// The returned transaction implements contracts.DBTX for SQLC compatibility.
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

// Close closes all database connections.
func Close() error {
	mu.RLock()
	defer mu.RUnlock()
	if instance == nil {
		return ErrNoInstance
	}
	return instance.Close()
}

// ErrNoInstance is returned when the database facade is not initialized.
var ErrNoInstance = &NoInstanceError{}

// NoInstanceError indicates the facade has not been initialized.
type NoInstanceError struct{}

func (e *NoInstanceError) Error() string {
	return "database facade not initialized: call db.SetInstance() first"
}
