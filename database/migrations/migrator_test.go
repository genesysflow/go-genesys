package migrations

import (
	"testing"

	"github.com/genesysflow/go-genesys/database"
	"github.com/genesysflow/go-genesys/database/schema"
	"github.com/genesysflow/go-genesys/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	_ "github.com/lib/pq"
)

// newTestDatabaseManager creates a database.Manager configured to use the test container.
func newTestDatabaseManager(pc *testutil.PostgresContainer) *database.Manager {
	cfg := database.Config{
		Default: "default",
		Connections: map[string]database.ConnectionConfig{
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
	return database.NewManager(cfg)
}

// testMigration implements Migration interface for testing
type testMigration struct {
	name string
	up   func(builder *schema.Builder) error
	down func(builder *schema.Builder) error
}

func (m *testMigration) Name() string {
	return m.name
}

func (m *testMigration) Up(builder *schema.Builder) error {
	if m.up != nil {
		return m.up(builder)
	}
	return nil
}

func (m *testMigration) Down(builder *schema.Builder) error {
	if m.down != nil {
		return m.down(builder)
	}
	return nil
}

func newTestMigration(name string, up, down func(builder *schema.Builder) error) *testMigration {
	return &testMigration{name: name, up: up, down: down}
}

func TestNewMigrator(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	pc, cleanup := testutil.SetupPostgresContainer(t)
	defer cleanup()

	manager := newTestDatabaseManager(pc)
	defer manager.Close()

	conn := manager.Connection()
	db := conn.DB()

	migrator := NewMigrator(db, "postgres", nil, nil)
	assert.NotNil(t, migrator)
}

func TestMigratorSetTable(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	pc, cleanup := testutil.SetupPostgresContainer(t)
	defer cleanup()

	manager := newTestDatabaseManager(pc)
	defer manager.Close()

	conn := manager.Connection()
	db := conn.DB()

	migrator := NewMigrator(db, "postgres", nil, nil)
	migrator.SetTable("custom_migrations")

	// Run should use custom table
	_, err := migrator.Run()
	require.NoError(t, err)

	// Verify custom table was created
	var exists int
	err = db.QueryRow(`
		SELECT COUNT(*) 
		FROM information_schema.tables 
		WHERE table_name = 'custom_migrations'
	`).Scan(&exists)
	require.NoError(t, err)
	assert.Equal(t, 1, exists)
}

func TestMigratorRun(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	pc, cleanup := testutil.SetupPostgresContainer(t)
	defer cleanup()

	manager := newTestDatabaseManager(pc)
	defer manager.Close()

	conn := manager.Connection()
	db := conn.DB()

	migrations := []Migration{
		newTestMigration("2024_01_01_000001_create_users", func(b *schema.Builder) error {
			return b.Create("users", func(bp *schema.Blueprint) {
				bp.ID()
				bp.String("name")
				bp.String("email").Unique()
				bp.Timestamps()
			})
		}, func(b *schema.Builder) error {
			return b.DropIfExists("users")
		}),
		newTestMigration("2024_01_01_000002_create_posts", func(b *schema.Builder) error {
			return b.Create("posts", func(bp *schema.Blueprint) {
				bp.ID()
				bp.String("title")
				bp.Text("body")
				bp.Integer("user_id")
				bp.Timestamps()
			})
		}, func(b *schema.Builder) error {
			return b.DropIfExists("posts")
		}),
	}

	migrator := NewMigrator(db, "postgres", migrations, nil)
	
	ran, err := migrator.Run()
	require.NoError(t, err)
	assert.Len(t, ran, 2)
	assert.Contains(t, ran, "2024_01_01_000001_create_users")
	assert.Contains(t, ran, "2024_01_01_000002_create_posts")

	// Verify tables were created
	builder := schema.NewBuilder(db, "postgres")
	assert.True(t, builder.HasTable("users"))
	assert.True(t, builder.HasTable("posts"))
}

func TestMigratorRunSkipsAlreadyRan(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	pc, cleanup := testutil.SetupPostgresContainer(t)
	defer cleanup()

	manager := newTestDatabaseManager(pc)
	defer manager.Close()

	conn := manager.Connection()
	db := conn.DB()

	migrations := []Migration{
		newTestMigration("2024_01_01_000001_create_test", func(b *schema.Builder) error {
			return b.Create("test_skip", func(bp *schema.Blueprint) {
				bp.ID()
			})
		}, func(b *schema.Builder) error {
			return b.DropIfExists("test_skip")
		}),
	}

	migrator := NewMigrator(db, "postgres", migrations, nil)

	// First run
	ran1, err := migrator.Run()
	require.NoError(t, err)
	assert.Len(t, ran1, 1)

	// Second run should skip
	ran2, err := migrator.Run()
	require.NoError(t, err)
	assert.Len(t, ran2, 0)
}

func TestMigratorRollback(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	pc, cleanup := testutil.SetupPostgresContainer(t)
	defer cleanup()

	manager := newTestDatabaseManager(pc)
	defer manager.Close()

	conn := manager.Connection()
	db := conn.DB()

	migrations := []Migration{
		newTestMigration("2024_01_01_000001_create_rollback_test", func(b *schema.Builder) error {
			return b.Create("rollback_test", func(bp *schema.Blueprint) {
				bp.ID()
				bp.String("name")
			})
		}, func(b *schema.Builder) error {
			return b.DropIfExists("rollback_test")
		}),
	}

	migrator := NewMigrator(db, "postgres", migrations, nil)

	// Run migration
	_, err := migrator.Run()
	require.NoError(t, err)

	builder := schema.NewBuilder(db, "postgres")
	assert.True(t, builder.HasTable("rollback_test"))

	// Rollback
	rolledBack, err := migrator.Rollback()
	require.NoError(t, err)
	assert.Len(t, rolledBack, 1)
	assert.Contains(t, rolledBack, "2024_01_01_000001_create_rollback_test")

	// Table should be dropped
	assert.False(t, builder.HasTable("rollback_test"))
}

func TestMigratorReset(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	pc, cleanup := testutil.SetupPostgresContainer(t)
	defer cleanup()

	manager := newTestDatabaseManager(pc)
	defer manager.Close()

	conn := manager.Connection()
	db := conn.DB()

	migrations := []Migration{
		newTestMigration("2024_01_01_000001_create_reset1", func(b *schema.Builder) error {
			return b.Create("reset1", func(bp *schema.Blueprint) {
				bp.ID()
			})
		}, func(b *schema.Builder) error {
			return b.DropIfExists("reset1")
		}),
		newTestMigration("2024_01_01_000002_create_reset2", func(b *schema.Builder) error {
			return b.Create("reset2", func(bp *schema.Blueprint) {
				bp.ID()
			})
		}, func(b *schema.Builder) error {
			return b.DropIfExists("reset2")
		}),
	}

	migrator := NewMigrator(db, "postgres", migrations, nil)

	// Run migrations
	_, err := migrator.Run()
	require.NoError(t, err)

	builder := schema.NewBuilder(db, "postgres")
	assert.True(t, builder.HasTable("reset1"))
	assert.True(t, builder.HasTable("reset2"))

	// Reset all
	allRolledBack, err := migrator.Reset()
	require.NoError(t, err)
	assert.Len(t, allRolledBack, 2)

	// All tables should be dropped
	assert.False(t, builder.HasTable("reset1"))
	assert.False(t, builder.HasTable("reset2"))
}

func TestMigratorStatus(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	pc, cleanup := testutil.SetupPostgresContainer(t)
	defer cleanup()

	manager := newTestDatabaseManager(pc)
	defer manager.Close()

	conn := manager.Connection()
	db := conn.DB()

	migrations := []Migration{
		newTestMigration("2024_01_01_000001_status_test", func(b *schema.Builder) error {
			return b.Create("status_test", func(bp *schema.Blueprint) {
				bp.ID()
			})
		}, func(b *schema.Builder) error {
			return b.DropIfExists("status_test")
		}),
		newTestMigration("2024_01_01_000002_status_test2", func(b *schema.Builder) error {
			return b.Create("status_test2", func(bp *schema.Blueprint) {
				bp.ID()
			})
		}, func(b *schema.Builder) error {
			return b.DropIfExists("status_test2")
		}),
	}

	migrator := NewMigrator(db, "postgres", migrations, nil)

	// Before running
	status, err := migrator.Status()
	require.NoError(t, err)
	assert.Len(t, status, 2)
	for _, s := range status {
		assert.False(t, s.Ran)
	}

	// Run first migration only
	migrator2 := NewMigrator(db, "postgres", migrations[:1], nil)
	_, err = migrator2.Run()
	require.NoError(t, err)

	// Check status with full migrator
	status, err = migrator.Status()
	require.NoError(t, err)

	var ran, notRan int
	for _, s := range status {
		if s.Ran {
			ran++
			assert.Equal(t, 1, s.Batch)
		} else {
			notRan++
		}
	}
	assert.Equal(t, 1, ran)
	assert.Equal(t, 1, notRan)
}

func TestMigratorBatchNumbers(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	pc, cleanup := testutil.SetupPostgresContainer(t)
	defer cleanup()

	manager := newTestDatabaseManager(pc)
	defer manager.Close()

	conn := manager.Connection()
	db := conn.DB()

	migration1 := newTestMigration("2024_01_01_000001_batch1", func(b *schema.Builder) error {
		return b.Create("batch1", func(bp *schema.Blueprint) {
			bp.ID()
		})
	}, func(b *schema.Builder) error {
		return b.DropIfExists("batch1")
	})

	migration2 := newTestMigration("2024_01_01_000002_batch2", func(b *schema.Builder) error {
		return b.Create("batch2", func(bp *schema.Blueprint) {
			bp.ID()
		})
	}, func(b *schema.Builder) error {
		return b.DropIfExists("batch2")
	})

	// Run first batch
	migrator1 := NewMigrator(db, "postgres", []Migration{migration1}, nil)
	_, err := migrator1.Run()
	require.NoError(t, err)

	// Run second batch
	migrator2 := NewMigrator(db, "postgres", []Migration{migration1, migration2}, nil)
	_, err = migrator2.Run()
	require.NoError(t, err)

	// Check status
	status, err := migrator2.Status()
	require.NoError(t, err)

	var batch1, batch2 int
	for _, s := range status {
		if s.Name == "2024_01_01_000001_batch1" {
			batch1 = s.Batch
		}
		if s.Name == "2024_01_01_000002_batch2" {
			batch2 = s.Batch
		}
	}
	assert.Equal(t, 1, batch1)
	assert.Equal(t, 2, batch2)
}

func TestMigratorBeforeAllMigrations(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	pc, cleanup := testutil.SetupPostgresContainer(t)
	defer cleanup()

	manager := newTestDatabaseManager(pc)
	defer manager.Close()

	conn := manager.Connection()
	db := conn.DB()

	beforeCalled := false
	beforeFn := func() error {
		beforeCalled = true
		return nil
	}

	migrations := []Migration{
		newTestMigration("2024_01_01_000001_before_test", func(b *schema.Builder) error {
			return b.Create("before_test", func(bp *schema.Blueprint) {
				bp.ID()
			})
		}, func(b *schema.Builder) error {
			return b.DropIfExists("before_test")
		}),
	}

	migrator := NewMigrator(db, "postgres", migrations, beforeFn)
	_, err := migrator.Run()
	require.NoError(t, err)
	assert.True(t, beforeCalled)
}

func TestMigratorRegister(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	pc, cleanup := testutil.SetupPostgresContainer(t)
	defer cleanup()

	manager := newTestDatabaseManager(pc)
	defer manager.Close()

	conn := manager.Connection()
	db := conn.DB()

	migrator := NewMigrator(db, "postgres", nil, nil)

	migrator.Register(newTestMigration("2024_01_01_000001_register_test", func(b *schema.Builder) error {
		return b.Create("register_test", func(bp *schema.Blueprint) {
			bp.ID()
		})
	}, func(b *schema.Builder) error {
		return b.DropIfExists("register_test")
	}))

	ran, err := migrator.Run()
	require.NoError(t, err)
	assert.Len(t, ran, 1)
}

func TestMigratorRegisterAll(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	pc, cleanup := testutil.SetupPostgresContainer(t)
	defer cleanup()

	manager := newTestDatabaseManager(pc)
	defer manager.Close()

	conn := manager.Connection()
	db := conn.DB()

	migrator := NewMigrator(db, "postgres", nil, nil)

	migrator.RegisterAll([]Migration{
		newTestMigration("2024_01_01_000001_register_all1", func(b *schema.Builder) error {
			return b.Create("register_all1", func(bp *schema.Blueprint) {
				bp.ID()
			})
		}, func(b *schema.Builder) error {
			return b.DropIfExists("register_all1")
		}),
		newTestMigration("2024_01_01_000002_register_all2", func(b *schema.Builder) error {
			return b.Create("register_all2", func(bp *schema.Blueprint) {
				bp.ID()
			})
		}, func(b *schema.Builder) error {
			return b.DropIfExists("register_all2")
		}),
	})

	ran, err := migrator.Run()
	require.NoError(t, err)
	assert.Len(t, ran, 2)
}

func TestPlaceholder(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	pc, cleanup := testutil.SetupPostgresContainer(t)
	defer cleanup()

	manager := newTestDatabaseManager(pc)
	defer manager.Close()

	conn := manager.Connection()
	db := conn.DB()

	// Test PostgreSQL placeholders
	pgMigrator := NewMigrator(db, "postgres", nil, nil)
	assert.Equal(t, "$1", pgMigrator.placeholder(1))
	assert.Equal(t, "$2", pgMigrator.placeholder(2))

	// Test MySQL/SQLite placeholders
	sqliteMigrator := NewMigrator(db, "sqlite", nil, nil)
	assert.Equal(t, "?", sqliteMigrator.placeholder(1))
	assert.Equal(t, "?", sqliteMigrator.placeholder(2))
}

func TestRollbackNothingToRollback(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	pc, cleanup := testutil.SetupPostgresContainer(t)
	defer cleanup()

	manager := newTestDatabaseManager(pc)
	defer manager.Close()

	conn := manager.Connection()
	db := conn.DB()

	migrator := NewMigrator(db, "postgres", nil, nil)

	// Running status to initialize migrations table
	_, _ = migrator.Status()

	// Rollback when nothing has been run
	rolledBack, err := migrator.Rollback()
	require.NoError(t, err)
	assert.Len(t, rolledBack, 0)
}
