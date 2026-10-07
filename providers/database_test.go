package providers

import (
	"testing"
	"time"

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

func TestParseConnectRetry(t *testing.T) {
	cases := []struct {
		in   any
		want time.Duration
	}{
		{nil, 0},
		{"", 0},
		{"15s", 15 * time.Second},
		{"500ms", 500 * time.Millisecond},
		{"-1s", -time.Second},
		{"20", 20 * time.Second},
		{30, 30 * time.Second},
		{int64(5), 5 * time.Second},
		{float64(2.5), 2500 * time.Millisecond},
		{-1, -time.Second},
	}
	for _, tc := range cases {
		got, err := parseConnectRetry(tc.in)
		require.NoError(t, err, "%v", tc.in)
		assert.Equal(t, tc.want, got, "%v", tc.in)
	}

	_, err := parseConnectRetry("soon")
	assert.Error(t, err)
	_, err = parseConnectRetry(true)
	assert.Error(t, err)
}

// connect_retry is read from config/database.yaml like the pool keys;
// a value that is not a duration fails boot instead of being ignored.
func TestDatabaseServiceProviderReadsConnectRetry(t *testing.T) {
	boot := func(v any) (*database.Manager, error) {
		cfg := testutil.NewMockConfig(map[string]any{
			"database.default": "default",
			"database.connections": map[string]any{
				"default": map[string]any{
					"driver":        "sqlite",
					"database":      ":memory:",
					"connect_retry": v,
				},
			},
		})
		app := testutil.NewMockApplicationWithConfig(cfg)
		provider := &DatabaseServiceProvider{}
		require.NoError(t, provider.Register(app))
		if err := provider.Boot(app); err != nil {
			return nil, err
		}
		return app.GetInstance("db").(*database.Manager), nil
	}

	m, err := boot("45s")
	require.NoError(t, err)
	cc, ok := m.GetConfig()
	require.True(t, ok)
	assert.Equal(t, 45*time.Second, cc.ConnectRetry)

	_, err = boot("whenever")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "connect_retry")
}
