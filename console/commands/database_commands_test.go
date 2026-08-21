package commands_test

import (
	"testing"

	"github.com/genesysflow/go-genesys/console/commands"
	"github.com/genesysflow/go-genesys/container"
	"github.com/genesysflow/go-genesys/database"
	"github.com/genesysflow/go-genesys/database/migrations"
	"github.com/genesysflow/go-genesys/database/schema"
	"github.com/genesysflow/go-genesys/events"
	"github.com/genesysflow/go-genesys/foundation"
	"github.com/genesysflow/go-genesys/providers"
	"github.com/genesysflow/go-genesys/schedule"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

// createUsers is a migration the inspection commands can find.
type createUsers struct{}

func (m *createUsers) Name() string { return "2026_01_01_000000_create_users_table" }

func (m *createUsers) Up(builder *schema.Builder) error {
	return builder.Create("users", func(table *schema.Blueprint) {
		table.ID()
		table.String("email", 255)
		table.Timestamps()
	})
}

func (m *createUsers) Down(builder *schema.Builder) error {
	return builder.Drop("users")
}

// dbApp boots an application on a shared in-memory sqlite database with
// one migration registered.
func dbApp(t *testing.T) *foundation.Application {
	t.Helper()

	app := foundation.New()
	require.NoError(t, app.Register(&providers.DatabaseServiceProvider{
		Config: &database.Config{
			Default: "test",
			Connections: map[string]database.ConnectionConfig{
				"test": {Driver: "sqlite", Database: "file:cmdtest?mode=memory&cache=shared"},
			},
		},
	}))
	require.NoError(t, app.Register(&providers.MigrationServiceProvider{
		Migrations: []migrations.Migration{&createUsers{}},
	}))
	require.NoError(t, app.Boot())

	t.Cleanup(func() {
		conn := container.MustResolve[*database.Manager](app).Connection()
		_, _ = conn.Exec("DROP TABLE IF EXISTS users")
		_, _ = conn.Exec("DROP TABLE IF EXISTS migrations")
	})

	return app
}

func TestMigrateRefreshCommand(t *testing.T) {
	app := dbApp(t)
	migrator := container.MustResolve[*migrations.Migrator](app)

	_, err := migrator.Run()
	require.NoError(t, err)

	conn := container.MustResolve[*database.Manager](app).Connection()
	_, err = conn.Exec("INSERT INTO users (email) VALUES ('ada@example.com')")
	require.NoError(t, err)

	out, err := runCommand(t, app, commands.MigrateRefreshCommand(app))
	require.NoError(t, err)
	assert.Contains(t, out, "create_users_table")

	// The table is back, and empty: refresh rolls back and re-runs.
	row := conn.QueryRow("SELECT COUNT(*) FROM users")
	var count int
	require.NoError(t, row.Scan(&count))
	assert.Equal(t, 0, count)

	status, err := migrator.Status()
	require.NoError(t, err)
	require.Len(t, status, 1)
	assert.True(t, status[0].Ran)
}

func TestDbWipeCommand(t *testing.T) {
	app := dbApp(t)
	migrator := container.MustResolve[*migrations.Migrator](app)
	_, err := migrator.Run()
	require.NoError(t, err)

	out, err := runCommand(t, app, commands.DbWipeCommand(app), "--force")
	require.NoError(t, err)
	assert.Contains(t, out, "users")

	conn := container.MustResolve[*database.Manager](app).Connection()
	_, err = conn.Query("SELECT * FROM users")
	assert.Error(t, err, "the table should be gone")
}

// db:wipe drops everything. In production it must not run on a bare
// invocation - a mistyped command should not be able to empty the
// database.
func TestDbWipeRefusesInProductionWithoutForce(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	app := dbApp(t)
	require.True(t, app.IsProduction())

	_, err := runCommand(t, app, commands.DbWipeCommand(app), "--no-interaction")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "production")
}

func TestDbShowCommand(t *testing.T) {
	app := dbApp(t)
	migrator := container.MustResolve[*migrations.Migrator](app)
	_, err := migrator.Run()
	require.NoError(t, err)

	conn := container.MustResolve[*database.Manager](app).Connection()
	_, err = conn.Exec("INSERT INTO users (email) VALUES ('ada@example.com')")
	require.NoError(t, err)

	out, err := runCommand(t, app, commands.DbShowCommand(app))
	require.NoError(t, err)

	assert.Contains(t, out, "sqlite")
	assert.Contains(t, out, "users")
	assert.Contains(t, out, "1", "the row count should be reported")
}

func TestDbTableCommand(t *testing.T) {
	app := dbApp(t)
	migrator := container.MustResolve[*migrations.Migrator](app)
	_, err := migrator.Run()
	require.NoError(t, err)

	out, err := runCommand(t, app, commands.DbTableCommand(app), "users")
	require.NoError(t, err)

	assert.Contains(t, out, "email")
	assert.Contains(t, out, "created_at")
}

func TestDbTableUnknownTable(t *testing.T) {
	app := dbApp(t)

	_, err := runCommand(t, app, commands.DbTableCommand(app), "nope")
	assert.Error(t, err)
}

// --- schedule:test ---------------------------------------------------

func TestScheduleTestCommand(t *testing.T) {
	app := foundation.New()

	ran := 0
	require.NoError(t, app.Register(&providers.ScheduleServiceProvider{
		Define: func(s *schedule.Schedule) {
			s.Call(func() error { ran++; return nil }).Daily().Description("nightly report")
			s.Call(func() error { return nil }).Hourly().Description("heartbeat")
		},
	}))
	require.NoError(t, app.Boot())

	out, err := runCommand(t, app, commands.ScheduleTestCommand(app), "nightly report")
	require.NoError(t, err)
	assert.Contains(t, out, "nightly report")
	assert.Equal(t, 1, ran, "the named task should run once, whether or not it is due")
}

func TestScheduleTestUnknownTask(t *testing.T) {
	app := foundation.New()
	require.NoError(t, app.Register(&providers.ScheduleServiceProvider{
		Define: func(s *schedule.Schedule) {
			s.Call(func() error { return nil }).Daily().Description("nightly report")
		},
	}))
	require.NoError(t, app.Boot())

	_, err := runCommand(t, app, commands.ScheduleTestCommand(app), "does not exist")
	assert.Error(t, err)
}

// --- event:list ------------------------------------------------------

func TestEventListCommand(t *testing.T) {
	app := foundation.New()
	require.NoError(t, app.Register(&providers.EventServiceProvider{}))
	require.NoError(t, app.Boot())

	dispatcher := container.MustResolve[*events.Dispatcher](app)
	dispatcher.Listen("user.registered", func(event events.Event) error { return nil })
	dispatcher.Listen("user.registered", func(event events.Event) error { return nil })
	dispatcher.Listen("order.placed", func(event events.Event) error { return nil })

	out, err := runCommand(t, app, commands.EventListCommand(app))
	require.NoError(t, err)

	assert.Contains(t, out, "user.registered")
	assert.Contains(t, out, "order.placed")
	assert.Contains(t, out, "2", "the listener count should be reported")
}
