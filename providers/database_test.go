package providers

import (
	"testing"

	"github.com/genesysflow/go-genesys/database"
	"github.com/genesysflow/go-genesys/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	_ "github.com/lib/pq"
)

func TestDatabaseServiceProviderRegister(t *testing.T) {
	app := testutil.NewMockApplication()
	provider := &DatabaseServiceProvider{}

	err := provider.Register(app)
	require.NoError(t, err)
	// Register should not bind anything, Boot does the work
}

func TestDatabaseServiceProviderBootWithConfig(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	pc, cleanup := testutil.SetupPostgresContainer(t)
	defer cleanup()

	cfg := testutil.NewMockConfig(map[string]any{
		"database.default": "default",
		"database.connections": map[string]any{
			"default": map[string]any{
				"driver":   "postgres",
				"host":     pc.Host,
				"port":     pc.Port,
				"database": pc.Database,
				"username": pc.Username,
				"password": pc.Password,
				"sslmode":  "disable",
			},
		},
	})
	app := testutil.NewMockApplicationWithConfig(cfg)
	provider := &DatabaseServiceProvider{}

	err := provider.Register(app)
	require.NoError(t, err)

	err = provider.Boot(app)
	require.NoError(t, err)

	// Check that database manager was registered
	dbManager := app.GetInstance("db")
	assert.NotNil(t, dbManager)
	assert.IsType(t, &database.Manager{}, dbManager)

	// Also check alias
	dbManager2 := app.GetInstance("database")
	assert.NotNil(t, dbManager2)
}

func TestDatabaseServiceProviderBootWithExplicitConfig(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	pc, cleanup := testutil.SetupPostgresContainer(t)
	defer cleanup()

	app := testutil.NewMockApplication()
	provider := &DatabaseServiceProvider{
		Config: &database.Config{
			Default: "default",
			Connections: map[string]database.ConnectionConfig{
				"default": {
					Driver:   "postgres",
					Host:     pc.Host,
					Port:     pc.Port,
					Database: pc.Database,
					Username: pc.Username,
					Password: pc.Password,
					SSLMode:  "disable",
				},
			},
		},
	}

	err := provider.Register(app)
	require.NoError(t, err)

	err = provider.Boot(app)
	require.NoError(t, err)

	dbManager := app.GetInstance("db")
	assert.NotNil(t, dbManager)

	// Test connection works
	manager := dbManager.(*database.Manager)
	conn := manager.Connection()
	require.NotNil(t, conn)

	err = conn.DB().Ping()
	require.NoError(t, err)
}

func TestDatabaseServiceProviderProvides(t *testing.T) {
	provider := &DatabaseServiceProvider{}
	provides := provider.Provides()

	// The provider should list its services
	assert.NotEmpty(t, provides)
}

func TestDatabaseServiceProviderMultipleConnections(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	pc, cleanup := testutil.SetupPostgresContainer(t)
	defer cleanup()

	cfg := testutil.NewMockConfig(map[string]any{
		"database.default": "primary",
		"database.connections": map[string]any{
			"primary": map[string]any{
				"driver":   "postgres",
				"host":     pc.Host,
				"port":     pc.Port,
				"database": pc.Database,
				"username": pc.Username,
				"password": pc.Password,
				"sslmode":  "disable",
			},
			"secondary": map[string]any{
				"driver":   "postgres",
				"host":     pc.Host,
				"port":     pc.Port,
				"database": pc.Database,
				"username": pc.Username,
				"password": pc.Password,
				"sslmode":  "disable",
			},
		},
	})
	app := testutil.NewMockApplicationWithConfig(cfg)
	provider := &DatabaseServiceProvider{}

	err := provider.Register(app)
	require.NoError(t, err)

	err = provider.Boot(app)
	require.NoError(t, err)

	manager := app.GetInstance("db").(*database.Manager)

	// Test primary connection
	primaryConn := manager.Connection("primary")
	require.NotNil(t, primaryConn)
	err = primaryConn.DB().Ping()
	require.NoError(t, err)

	// Test secondary connection
	secondaryConn := manager.Connection("secondary")
	require.NotNil(t, secondaryConn)
	err = secondaryConn.DB().Ping()
	require.NoError(t, err)
}

func TestDatabaseServiceProviderWithConnectionPool(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	pc, cleanup := testutil.SetupPostgresContainer(t)
	defer cleanup()

	cfg := testutil.NewMockConfig(map[string]any{
		"database.default": "default",
		"database.connections": map[string]any{
			"default": map[string]any{
				"driver":         "postgres",
				"host":           pc.Host,
				"port":           pc.Port,
				"database":       pc.Database,
				"username":       pc.Username,
				"password":       pc.Password,
				"sslmode":        "disable",
				"max_open_conns": 10,
				"max_idle_conns": 5,
			},
		},
	})
	app := testutil.NewMockApplicationWithConfig(cfg)
	provider := &DatabaseServiceProvider{}

	err := provider.Register(app)
	require.NoError(t, err)

	err = provider.Boot(app)
	require.NoError(t, err)

	manager := app.GetInstance("db").(*database.Manager)
	conn := manager.Connection()
	require.NotNil(t, conn)

	err = conn.DB().Ping()
	require.NoError(t, err)
}
