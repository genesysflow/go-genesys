package database

import (
	"errors"
	"testing"

	"github.com/genesysflow/go-genesys/contracts"
	"github.com/genesysflow/go-genesys/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	_ "github.com/lib/pq"
)

// newTestDatabaseManager creates a database.Manager configured to use the test container.
func newTestDatabaseManager(pc *testutil.PostgresContainer) *Manager {
	cfg := Config{
		Default: "default",
		Connections: map[string]ConnectionConfig{
			"default": {
				Driver:   "pgsql",
				Host:     pc.Host,
				Port:     pc.Port,
				Database: pc.Database,
				Username: pc.Username,
				Password: pc.Password,
				SSLMode:  "disable",
			},
		},
	}
	return NewManager(cfg)
}

func TestNewManager(t *testing.T) {
	cfg := Config{
		Default: "default",
		Connections: map[string]ConnectionConfig{
			"default": {
				Driver:   "pgsql",
				Host:     "localhost",
				Port:     5432,
				Database: "testdb",
				Username: "user",
				Password: "pass",
			},
		},
	}

	manager := NewManager(cfg)
	assert.NotNil(t, manager)
	assert.Equal(t, "default", manager.GetDefaultConnection())
}

func TestGetConfig(t *testing.T) {
	cfg := Config{
		Default: "primary",
		Connections: map[string]ConnectionConfig{
			"primary": {
				Driver:   "pgsql",
				Host:     "primary.example.com",
				Port:     5432,
				Database: "primary_db",
			},
			"secondary": {
				Driver:   "pgsql",
				Host:     "secondary.example.com",
				Port:     5433,
				Database: "secondary_db",
			},
		},
	}

	manager := NewManager(cfg)

	// Get default connection config
	connCfg, ok := manager.GetConfig()
	assert.True(t, ok)
	assert.Equal(t, "primary.example.com", connCfg.Host)

	// Get named connection config
	connCfg, ok = manager.GetConfig("secondary")
	assert.True(t, ok)
	assert.Equal(t, "secondary.example.com", connCfg.Host)

	// Get non-existent connection
	_, ok = manager.GetConfig("nonexistent")
	assert.False(t, ok)
}

func TestSetDefaultConnection(t *testing.T) {
	cfg := Config{
		Default: "primary",
		Connections: map[string]ConnectionConfig{
			"primary":   {},
			"secondary": {},
		},
	}

	manager := NewManager(cfg)
	assert.Equal(t, "primary", manager.GetDefaultConnection())

	manager.SetDefaultConnection("secondary")
	assert.Equal(t, "secondary", manager.GetDefaultConnection())
}

func TestBuildDSN(t *testing.T) {
	testCases := []struct {
		name     string
		config   ConnectionConfig
		expected string
	}{
		{
			name: "postgresql",
			config: ConnectionConfig{
				Driver:   "pgsql",
				Host:     "localhost",
				Port:     5432,
				Database: "mydb",
				Username: "user",
				Password: "pass",
				SSLMode:  "disable",
			},
			expected: "host=localhost port=5432 user=user password=pass dbname=mydb sslmode=disable",
		},
		{
			name: "postgresql with postgres driver",
			config: ConnectionConfig{
				Driver:   "postgres",
				Host:     "db.example.com",
				Port:     5433,
				Database: "proddb",
				Username: "admin",
				Password: "secret",
				SSLMode:  "require",
			},
			expected: "host=db.example.com port=5433 user=admin password=secret dbname=proddb sslmode=require",
		},
		{
			name: "postgresql default sslmode",
			config: ConnectionConfig{
				Driver:   "pgsql",
				Host:     "localhost",
				Port:     5432,
				Database: "mydb",
				Username: "user",
				Password: "pass",
			},
			expected: "host=localhost port=5432 user=user password=pass dbname=mydb sslmode=disable",
		},
		{
			name: "postgresql default port",
			config: ConnectionConfig{
				Driver:   "pgsql",
				Host:     "localhost",
				Database: "mydb",
				Username: "user",
				Password: "pass",
			},
			expected: "host=localhost port=5432 user=user password=pass dbname=mydb sslmode=disable",
		},
		{
			name: "sqlite",
			config: ConnectionConfig{
				Driver:   "sqlite",
				Database: "/path/to/database.db",
			},
			expected: "/path/to/database.db",
		},
		{
			name: "sqlite3",
			config: ConnectionConfig{
				Driver:   "sqlite3",
				Database: ":memory:",
			},
			expected: ":memory:",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := buildDSN(tc.config)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestMapDriver(t *testing.T) {
	testCases := []struct {
		input    string
		expected string
	}{
		{"pgsql", "postgres"},
		{"postgres", "postgres"},
		{"postgresql", "postgres"},
		{"sqlite", "sqlite"},
		{"sqlite3", "sqlite"},
		{"mysql", "mysql"},
		{"unknown", "unknown"},
	}

	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			result := mapDriver(tc.input)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestConnectionMissingConfig(t *testing.T) {
	cfg := Config{
		Default:     "default",
		Connections: map[string]ConnectionConfig{},
	}

	manager := NewManager(cfg)
	conn := manager.Connection()

	// Connection should have error state
	assert.NotNil(t, conn.Error())
	assert.Contains(t, conn.Error().Error(), "not configured")
}

// Integration tests using testcontainers-go
// These tests require Docker to be running

func TestConnectionIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	pc, cleanup := testutil.SetupPostgresContainer(t)
	defer cleanup()

	manager := newTestDatabaseManager(pc)
	defer manager.Close()

	conn := manager.Connection()
	assert.NotNil(t, conn)
	assert.NoError(t, conn.Error())
	assert.Equal(t, "pgsql", conn.Driver())
	assert.Equal(t, "default", conn.Name())
}

func TestConnectionCaching(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	pc, cleanup := testutil.SetupPostgresContainer(t)
	defer cleanup()

	manager := newTestDatabaseManager(pc)
	defer manager.Close()

	conn1 := manager.Connection()
	conn2 := manager.Connection()

	// Should be the same instance
	assert.Equal(t, conn1, conn2)
}

func TestConnectionPing(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	pc, cleanup := testutil.SetupPostgresContainer(t)
	defer cleanup()

	manager := newTestDatabaseManager(pc)
	defer manager.Close()

	conn := manager.Connection()
	err := conn.Ping()
	assert.NoError(t, err)
}

func TestConnectionQuery(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	pc, cleanup := testutil.SetupPostgresContainer(t)
	defer cleanup()

	manager := newTestDatabaseManager(pc)
	defer manager.Close()

	conn := manager.Connection()

	rows, err := conn.Query("SELECT 1 + 1 AS result")
	require.NoError(t, err)
	defer rows.Close()

	var result int
	require.True(t, rows.Next())
	err = rows.Scan(&result)
	require.NoError(t, err)
	assert.Equal(t, 2, result)
}

func TestConnectionExec(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	pc, cleanup := testutil.SetupPostgresContainer(t)
	defer cleanup()

	manager := newTestDatabaseManager(pc)
	defer manager.Close()

	conn := manager.Connection()

	// Create a table
	_, err := conn.Exec(`
		CREATE TABLE IF NOT EXISTS test_users (
			id SERIAL PRIMARY KEY,
			name VARCHAR(255) NOT NULL
		)
	`)
	require.NoError(t, err)

	// Insert a row
	result, err := conn.Exec("INSERT INTO test_users (name) VALUES ($1)", "John")
	require.NoError(t, err)

	rowsAffected, err := result.RowsAffected()
	require.NoError(t, err)
	assert.Equal(t, int64(1), rowsAffected)
}

func TestManagerRaw(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	pc, cleanup := testutil.SetupPostgresContainer(t)
	defer cleanup()

	manager := newTestDatabaseManager(pc)
	defer manager.Close()

	rows, err := manager.Raw("SELECT 42 AS answer")
	require.NoError(t, err)
	defer rows.Close()

	var answer int
	require.True(t, rows.Next())
	err = rows.Scan(&answer)
	require.NoError(t, err)
	assert.Equal(t, 42, answer)
}

func TestTransactionCommit(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	pc, cleanup := testutil.SetupPostgresContainer(t)
	defer cleanup()

	manager := newTestDatabaseManager(pc)
	defer manager.Close()

	conn := manager.Connection()

	// Create test table
	_, err := conn.Exec(`
		CREATE TABLE IF NOT EXISTS tx_test (
			id SERIAL PRIMARY KEY,
			value VARCHAR(255)
		)
	`)
	require.NoError(t, err)

	// Run transaction
	err = conn.Transaction(func(tx contracts.Transaction) error {
		_, err := tx.Exec("INSERT INTO tx_test (value) VALUES ($1)", "committed")
		return err
	})
	require.NoError(t, err)

	// Verify data was committed
	rows, err := conn.Query("SELECT value FROM tx_test WHERE value = $1", "committed")
	require.NoError(t, err)
	defer rows.Close()
	assert.True(t, rows.Next())
}

func TestTransactionRollback(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	pc, cleanup := testutil.SetupPostgresContainer(t)
	defer cleanup()

	manager := newTestDatabaseManager(pc)
	defer manager.Close()

	conn := manager.Connection()

	// Create test table
	_, err := conn.Exec(`
		CREATE TABLE IF NOT EXISTS rollback_test (
			id SERIAL PRIMARY KEY,
			value VARCHAR(255)
		)
	`)
	require.NoError(t, err)

	// Run transaction that will rollback
	expectedErr := errors.New("intentional error")
	err = conn.Transaction(func(tx contracts.Transaction) error {
		_, _ = tx.Exec("INSERT INTO rollback_test (value) VALUES ($1)", "should-rollback")
		return expectedErr
	})
	assert.Equal(t, expectedErr, err)

	// Verify data was NOT committed
	rows, err := conn.Query("SELECT value FROM rollback_test WHERE value = $1", "should-rollback")
	require.NoError(t, err)
	defer rows.Close()
	assert.False(t, rows.Next())
}

func TestBeginTransaction(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	pc, cleanup := testutil.SetupPostgresContainer(t)
	defer cleanup()

	manager := newTestDatabaseManager(pc)
	defer manager.Close()

	conn := manager.Connection()

	// Create test table
	_, err := conn.Exec(`
		CREATE TABLE IF NOT EXISTS manual_tx_test (
			id SERIAL PRIMARY KEY,
			value VARCHAR(255)
		)
	`)
	require.NoError(t, err)

	tx, err := conn.BeginTransaction()
	require.NoError(t, err)

	_, err = tx.Exec("INSERT INTO manual_tx_test (value) VALUES ($1)", "manual-tx")
	require.NoError(t, err)

	err = tx.Commit()
	require.NoError(t, err)

	// Verify
	rows, err := conn.Query("SELECT value FROM manual_tx_test WHERE value = $1", "manual-tx")
	require.NoError(t, err)
	defer rows.Close()
	assert.True(t, rows.Next())
}

func TestDisconnect(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	pc, cleanup := testutil.SetupPostgresContainer(t)
	defer cleanup()

	manager := newTestDatabaseManager(pc)
	defer manager.Close()

	// Get connection to create it
	conn := manager.Connection()
	require.NoError(t, conn.Error())

	// Disconnect
	err := manager.Disconnect()
	require.NoError(t, err)

	// Getting connection again should create new one
	conn2 := manager.Connection()
	require.NoError(t, conn2.Error())
}

func TestReconnect(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	pc, cleanup := testutil.SetupPostgresContainer(t)
	defer cleanup()

	manager := newTestDatabaseManager(pc)
	defer manager.Close()

	conn1 := manager.Connection()
	require.NoError(t, conn1.Error())

	conn2, err := manager.Reconnect()
	require.NoError(t, err)
	require.NoError(t, conn2.Error())

	// Should be a different instance
	assert.NotEqual(t, conn1, conn2)
}

func TestMultipleConnections(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	pc, cleanup := testutil.SetupPostgresContainer(t)
	defer cleanup()

	cfg := Config{
		Default: "primary",
		Connections: map[string]ConnectionConfig{
			"primary": {
				Driver:   "pgsql",
				Host:     pc.Host,
				Port:     pc.Port,
				Database: pc.Database,
				Username: pc.Username,
				Password: pc.Password,
				SSLMode:  "disable",
			},
			"secondary": {
				Driver:   "pgsql",
				Host:     pc.Host,
				Port:     pc.Port,
				Database: pc.Database,
				Username: pc.Username,
				Password: pc.Password,
				SSLMode:  "disable",
			},
		},
	}

	manager := NewManager(cfg)
	defer manager.Close()

	primary := manager.Connection("primary")
	secondary := manager.Connection("secondary")

	require.NoError(t, primary.Error())
	require.NoError(t, secondary.Error())

	assert.Equal(t, "primary", primary.Name())
	assert.Equal(t, "secondary", secondary.Name())
}

func TestCloseAll(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	pc, cleanup := testutil.SetupPostgresContainer(t)
	defer cleanup()

	manager := newTestDatabaseManager(pc)

	// Create connection
	conn := manager.Connection()
	require.NoError(t, conn.Error())

	// Close all
	err := manager.Close()
	require.NoError(t, err)
}

func TestConnectionDB(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	pc, cleanup := testutil.SetupPostgresContainer(t)
	defer cleanup()

	manager := newTestDatabaseManager(pc)
	defer manager.Close()

	conn := manager.Connection()
	db := conn.DB()

	assert.NotNil(t, db)
	err := db.Ping()
	assert.NoError(t, err)
}
