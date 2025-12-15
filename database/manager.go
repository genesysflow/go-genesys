// Package database provides a Laravel-inspired database layer.
package database

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	"github.com/genesysflow/go-genesys/contracts"
	"github.com/genesysflow/go-genesys/database/query"
)

// Config represents database configuration.
type Config struct {
	// Default connection name.
	Default string `yaml:"default" json:"default"`

	// Connections defines all database connections.
	Connections map[string]ConnectionConfig `yaml:"connections" json:"connections"`
}

// ConnectionConfig represents a single database connection configuration.
type ConnectionConfig struct {
	// Driver is the database driver (pgsql, sqlite).
	Driver string `yaml:"driver" json:"driver"`

	// Host is the database host.
	Host string `yaml:"host" json:"host"`

	// Port is the database port.
	Port int `yaml:"port" json:"port"`

	// Database is the database name.
	Database string `yaml:"database" json:"database"`

	// Username for authentication.
	Username string `yaml:"username" json:"username"`

	// Password for authentication.
	Password string `yaml:"password" json:"password"`

	// SSLMode for PostgreSQL connections.
	SSLMode string `yaml:"sslmode" json:"sslmode"`

	// MaxOpenConns sets the maximum open connections.
	MaxOpenConns int `yaml:"max_open_conns" json:"max_open_conns"`

	// MaxIdleConns sets the maximum idle connections.
	MaxIdleConns int `yaml:"max_idle_conns" json:"max_idle_conns"`

	// ConnMaxLifetime is the maximum connection lifetime.
	ConnMaxLifetime time.Duration `yaml:"conn_max_lifetime" json:"conn_max_lifetime"`

	// ConnMaxIdleTime is the maximum idle time for connections.
	ConnMaxIdleTime time.Duration `yaml:"conn_max_idle_time" json:"conn_max_idle_time"`

	// Prefix for table names.
	Prefix string `yaml:"prefix" json:"prefix"`

	// ForeignKeyConstraints enables foreign key constraints (SQLite).
	ForeignKeyConstraints bool `yaml:"foreign_key_constraints" json:"foreign_key_constraints"`
}

// Manager is the database manager that handles multiple connections.
type Manager struct {
	config      Config
	connections map[string]*Connection
	mu          sync.RWMutex
}

// NewManager creates a new database manager.
func NewManager(config Config) *Manager {
	return &Manager{
		config:      config,
		connections: make(map[string]*Connection),
	}
}

// Connection returns a connection by name.
// If no name is provided, the default connection is returned.
func (m *Manager) Connection(name ...string) contracts.Connection {
	connName := m.config.Default
	if len(name) > 0 && name[0] != "" {
		connName = name[0]
	}

	m.mu.RLock()
	if conn, ok := m.connections[connName]; ok {
		m.mu.RUnlock()
		return conn
	}
	m.mu.RUnlock()

	// Create new connection
	conn, err := m.makeConnection(connName)
	if err != nil {
		// Return nil connection - will cause errors on use
		return nil
	}

	m.mu.Lock()
	m.connections[connName] = conn
	m.mu.Unlock()

	return conn
}

// makeConnection creates a new database connection.
func (m *Manager) makeConnection(name string) (*Connection, error) {
	config, ok := m.config.Connections[name]
	if !ok {
		return nil, fmt.Errorf("database connection [%s] not configured", name)
	}

	dsn := buildDSN(config)
	driverName := mapDriver(config.Driver)

	db, err := sql.Open(driverName, dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Configure connection pool
	if config.MaxOpenConns > 0 {
		db.SetMaxOpenConns(config.MaxOpenConns)
	}
	if config.MaxIdleConns > 0 {
		db.SetMaxIdleConns(config.MaxIdleConns)
	}
	if config.ConnMaxLifetime > 0 {
		db.SetConnMaxLifetime(config.ConnMaxLifetime)
	}
	if config.ConnMaxIdleTime > 0 {
		db.SetConnMaxIdleTime(config.ConnMaxIdleTime)
	}

	// Verify connection
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	// Enable foreign keys for SQLite
	if config.Driver == "sqlite" && config.ForeignKeyConstraints {
		_, _ = db.Exec("PRAGMA foreign_keys = ON")
	}

	return &Connection{
		name:    name,
		driver:  config.Driver,
		db:      db,
		prefix:  config.Prefix,
		grammar: query.NewGrammar(config.Driver),
	}, nil
}

// Table starts a query builder for the given table.
func (m *Manager) Table(table string) contracts.QueryBuilder {
	conn := m.Connection()
	if conn == nil {
		return nil
	}
	return conn.Table(table)
}

// Raw executes a raw SQL query.
func (m *Manager) Raw(sqlQuery string, bindings ...any) (*sql.Rows, error) {
	conn := m.Connection()
	if conn == nil {
		return nil, fmt.Errorf("no database connection available")
	}
	return conn.Query(sqlQuery, bindings...)
}

// Select executes a raw select query.
func (m *Manager) Select(sqlQuery string, bindings ...any) (*sql.Rows, error) {
	return m.Raw(sqlQuery, bindings...)
}

// Insert executes a raw insert query.
func (m *Manager) Insert(sqlQuery string, bindings ...any) (sql.Result, error) {
	conn := m.Connection()
	if conn == nil {
		return nil, fmt.Errorf("no database connection available")
	}
	return conn.Exec(sqlQuery, bindings...)
}

// Update executes a raw update query.
func (m *Manager) Update(sqlQuery string, bindings ...any) (sql.Result, error) {
	return m.Insert(sqlQuery, bindings...)
}

// Delete executes a raw delete query.
func (m *Manager) Delete(sqlQuery string, bindings ...any) (sql.Result, error) {
	return m.Insert(sqlQuery, bindings...)
}

// Statement executes a raw statement.
func (m *Manager) Statement(sqlQuery string, bindings ...any) (sql.Result, error) {
	return m.Insert(sqlQuery, bindings...)
}

// Transaction runs a callback in a database transaction.
func (m *Manager) Transaction(fn func(tx contracts.Transaction) error) error {
	conn := m.Connection()
	if conn == nil {
		return fmt.Errorf("no database connection available")
	}
	return conn.Transaction(fn)
}

// BeginTransaction starts a new database transaction.
func (m *Manager) BeginTransaction() (contracts.Transaction, error) {
	conn := m.Connection()
	if conn == nil {
		return nil, fmt.Errorf("no database connection available")
	}
	return conn.BeginTransaction()
}

// GetDefaultConnection returns the default connection name.
func (m *Manager) GetDefaultConnection() string {
	return m.config.Default
}

// SetDefaultConnection sets the default connection name.
func (m *Manager) SetDefaultConnection(name string) {
	m.config.Default = name
}

// Disconnect disconnects from the given connection.
func (m *Manager) Disconnect(name ...string) error {
	connName := m.config.Default
	if len(name) > 0 && name[0] != "" {
		connName = name[0]
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if conn, ok := m.connections[connName]; ok {
		if err := conn.Close(); err != nil {
			return err
		}
		delete(m.connections, connName)
	}

	return nil
}

// Reconnect reconnects to the given connection.
func (m *Manager) Reconnect(name ...string) (contracts.Connection, error) {
	connName := m.config.Default
	if len(name) > 0 && name[0] != "" {
		connName = name[0]
	}

	// Disconnect first
	_ = m.Disconnect(connName)

	// Create new connection
	conn, err := m.makeConnection(connName)
	if err != nil {
		return nil, err
	}

	m.mu.Lock()
	m.connections[connName] = conn
	m.mu.Unlock()

	return conn, nil
}

// Close closes all connections.
func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var lastErr error
	for name, conn := range m.connections {
		if err := conn.Close(); err != nil {
			lastErr = err
		}
		delete(m.connections, name)
	}

	return lastErr
}

// buildDSN builds a connection string from configuration.
func buildDSN(config ConnectionConfig) string {
	switch config.Driver {
	case "pgsql", "postgres", "postgresql":
		sslMode := config.SSLMode
		if sslMode == "" {
			sslMode = "disable"
		}
		if config.Port == 0 {
			config.Port = 5432
		}
		return fmt.Sprintf(
			"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
			config.Host, config.Port, config.Username, config.Password, config.Database, sslMode,
		)

	case "sqlite", "sqlite3":
		return config.Database

	default:
		return ""
	}
}

// mapDriver maps driver names to Go sql driver names.
func mapDriver(driver string) string {
	switch driver {
	case "pgsql", "postgres", "postgresql":
		return "postgres"
	case "sqlite", "sqlite3":
		return "sqlite3"
	default:
		return driver
	}
}

// Connection represents a database connection.
type Connection struct {
	name    string
	driver  string
	db      *sql.DB
	prefix  string
	grammar contracts.Grammar
}

// Name returns the connection name.
func (c *Connection) Name() string {
	return c.name
}

// Driver returns the driver name.
func (c *Connection) Driver() string {
	return c.driver
}

// DB returns the underlying *sql.DB.
func (c *Connection) DB() *sql.DB {
	return c.db
}

// Table starts a query builder for the given table.
func (c *Connection) Table(table string) contracts.QueryBuilder {
	return query.NewBuilder(c.db, c.grammar, c.prefix+table)
}

// Query executes a raw query.
func (c *Connection) Query(sqlQuery string, bindings ...any) (*sql.Rows, error) {
	return c.db.Query(sqlQuery, bindings...)
}

// Exec executes a raw statement.
func (c *Connection) Exec(sqlQuery string, bindings ...any) (sql.Result, error) {
	return c.db.Exec(sqlQuery, bindings...)
}

// Prepare prepares a statement.
func (c *Connection) Prepare(sqlQuery string) (*sql.Stmt, error) {
	return c.db.Prepare(sqlQuery)
}

// BeginTransaction starts a transaction.
func (c *Connection) BeginTransaction() (contracts.Transaction, error) {
	tx, err := c.db.Begin()
	if err != nil {
		return nil, err
	}
	return &Transaction{tx: tx, grammar: c.grammar, prefix: c.prefix}, nil
}

// Transaction runs a callback in a transaction.
func (c *Connection) Transaction(fn func(tx contracts.Transaction) error) error {
	tx, err := c.BeginTransaction()
	if err != nil {
		return err
	}

	defer func() {
		if r := recover(); r != nil {
			_ = tx.Rollback()
			panic(r)
		}
	}()

	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}

	return tx.Commit()
}

// Close closes the connection.
func (c *Connection) Close() error {
	return c.db.Close()
}

// Ping verifies the connection is alive.
func (c *Connection) Ping() error {
	return c.db.Ping()
}

// PingContext verifies the connection is alive with context.
func (c *Connection) PingContext(ctx context.Context) error {
	return c.db.PingContext(ctx)
}

// Transaction represents an active database transaction.
type Transaction struct {
	tx      *sql.Tx
	grammar contracts.Grammar
	prefix  string
}

// Query executes a query within the transaction.
func (t *Transaction) Query(sqlQuery string, bindings ...any) (*sql.Rows, error) {
	return t.tx.Query(sqlQuery, bindings...)
}

// Exec executes a statement within the transaction.
func (t *Transaction) Exec(sqlQuery string, bindings ...any) (sql.Result, error) {
	return t.tx.Exec(sqlQuery, bindings...)
}

// Table starts a query builder within the transaction.
func (t *Transaction) Table(table string) contracts.QueryBuilder {
	return query.NewBuilderWithTx(t.tx, t.grammar, t.prefix+table)
}

// Commit commits the transaction.
func (t *Transaction) Commit() error {
	return t.tx.Commit()
}

// Rollback rolls back the transaction.
func (t *Transaction) Rollback() error {
	return t.tx.Rollback()
}
