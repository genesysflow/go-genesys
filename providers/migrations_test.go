package providers

import (
	"testing"

	"github.com/genesysflow/go-genesys/database"
	"github.com/genesysflow/go-genesys/database/migrations"
	"github.com/genesysflow/go-genesys/database/schema"
	"github.com/genesysflow/go-genesys/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	_ "github.com/lib/pq"
)

// testProviderMigration implements migrations.Migration for testing.
type testProviderMigration struct {
	name string
}

func (m *testProviderMigration) Name() string {
	return m.name
}

func (m *testProviderMigration) Up(builder *schema.Builder) error {
	return builder.Create("test_provider_table", func(bp *schema.Blueprint) {
		bp.ID()
		bp.String("name")
	})
}

func (m *testProviderMigration) Down(builder *schema.Builder) error {
	return builder.Drop("test_provider_table")
}

func TestMigrationServiceProviderRegister(t *testing.T) {
	app := testutil.NewMockApplication()
	provider := &MigrationServiceProvider{}

	err := provider.Register(app)
	require.NoError(t, err)
	// Register just stores the app reference
}

func TestMigrationServiceProviderBootWithDatabase(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	pc, cleanup := testutil.SetupPostgresContainer(t)
	defer cleanup()

	// First, set up the database provider
	dbConfig := &database.Config{
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
	}

	app := testutil.NewMockApplication()

	// Register and boot database provider first
	dbProvider := &DatabaseServiceProvider{Config: dbConfig}
	err := dbProvider.Register(app)
	require.NoError(t, err)
	err = dbProvider.Boot(app)
	require.NoError(t, err)

	// Now register and boot migration provider
	migrationsProvider := &MigrationServiceProvider{
		Migrations: []migrations.Migration{
			&testProviderMigration{name: "001_create_test_table"},
		},
	}

	err = migrationsProvider.Register(app)
	require.NoError(t, err)

	err = migrationsProvider.Boot(app)
	require.NoError(t, err)

	// Check that migrator was registered
	migrator := app.GetInstance("migrator")
	assert.NotNil(t, migrator)
	assert.IsType(t, &migrations.Migrator{}, migrator)
}

func TestMigrationServiceProviderBootWithBeforeHook(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	pc, cleanup := testutil.SetupPostgresContainer(t)
	defer cleanup()

	dbConfig := &database.Config{
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
	}

	app := testutil.NewMockApplication()

	// Setup database first
	dbProvider := &DatabaseServiceProvider{Config: dbConfig}
	err := dbProvider.Register(app)
	require.NoError(t, err)
	err = dbProvider.Boot(app)
	require.NoError(t, err)

	hookCalled := false
	migrationsProvider := &MigrationServiceProvider{
		BeforeAllMigrations: func() error {
			hookCalled = true
			return nil
		},
		Migrations: []migrations.Migration{
			&testProviderMigration{name: "001_create_test_table"},
		},
	}

	err = migrationsProvider.Register(app)
	require.NoError(t, err)

	err = migrationsProvider.Boot(app)
	require.NoError(t, err)

	// The hook won't be called until migrations are run
	// but the provider should be set up correctly
	migrator := app.GetInstance("migrator").(*migrations.Migrator)
	assert.NotNil(t, migrator)

	// Run migrations to trigger the hook
	_, err = migrator.Run()
	require.NoError(t, err)

	assert.True(t, hookCalled)
}

func TestMigrationServiceProviderProvides(t *testing.T) {
	provider := &MigrationServiceProvider{}
	provides := provider.Provides()

	assert.Contains(t, provides, "migrator")
}

func TestMigrationServiceProviderRunMigrations(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	pc, cleanup := testutil.SetupPostgresContainer(t)
	defer cleanup()

	dbConfig := &database.Config{
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
	}

	app := testutil.NewMockApplication()

	// Setup database
	dbProvider := &DatabaseServiceProvider{Config: dbConfig}
	err := dbProvider.Register(app)
	require.NoError(t, err)
	err = dbProvider.Boot(app)
	require.NoError(t, err)

	// Setup migrations
	migrationsProvider := &MigrationServiceProvider{
		Migrations: []migrations.Migration{
			&testProviderMigration{name: "001_create_test_table"},
		},
	}

	err = migrationsProvider.Register(app)
	require.NoError(t, err)
	err = migrationsProvider.Boot(app)
	require.NoError(t, err)

	// Run migrations
	migrator := app.GetInstance("migrator").(*migrations.Migrator)
	_, err = migrator.Run()
	require.NoError(t, err)

	// Verify table was created
	manager := app.GetInstance("db").(*database.Manager)
	conn := manager.Connection()
	db := conn.DB()

	var exists bool
	err = db.QueryRow(`
		SELECT EXISTS (
			SELECT FROM information_schema.tables 
			WHERE table_name = 'test_provider_table'
		)
	`).Scan(&exists)
	require.NoError(t, err)
	assert.True(t, exists)
}

func TestMigrationServiceProviderRollback(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	pc, cleanup := testutil.SetupPostgresContainer(t)
	defer cleanup()

	dbConfig := &database.Config{
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
	}

	app := testutil.NewMockApplication()

	// Setup database
	dbProvider := &DatabaseServiceProvider{Config: dbConfig}
	err := dbProvider.Register(app)
	require.NoError(t, err)
	err = dbProvider.Boot(app)
	require.NoError(t, err)

	// Setup migrations
	migrationsProvider := &MigrationServiceProvider{
		Migrations: []migrations.Migration{
			&testProviderMigration{name: "001_create_test_table"},
		},
	}

	err = migrationsProvider.Register(app)
	require.NoError(t, err)
	err = migrationsProvider.Boot(app)
	require.NoError(t, err)

	migrator := app.GetInstance("migrator").(*migrations.Migrator)

	// Run migrations
	_, err = migrator.Run()
	require.NoError(t, err)

	// Rollback
	_, err = migrator.Rollback()
	require.NoError(t, err)

	// Verify table was dropped
	manager := app.GetInstance("db").(*database.Manager)
	conn := manager.Connection()
	db := conn.DB()

	var exists bool
	err = db.QueryRow(`
		SELECT EXISTS (
			SELECT FROM information_schema.tables 
			WHERE table_name = 'test_provider_table'
		)
	`).Scan(&exists)
	require.NoError(t, err)
	assert.False(t, exists)
}
