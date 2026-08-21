package commands

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/genesysflow/go-genesys/database/migrations"
	"github.com/genesysflow/go-genesys/database/schema"
	"github.com/genesysflow/go-genesys/database/seed"
	"github.com/genesysflow/go-genesys/queue"
	"github.com/genesysflow/go-genesys/schedule"
	"github.com/genesysflow/go-genesys/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

func TestWriteAppKey(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")

	// Creates the file when missing.
	require.NoError(t, writeAppKey(path, "base64:first"))
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "APP_KEY=base64:first\n", string(content))

	// Replaces an existing APP_KEY line, keeping other lines.
	require.NoError(t, os.WriteFile(path, []byte("APP_ENV=local\nAPP_KEY=old\nDB=x\n"), 0o600))
	require.NoError(t, writeAppKey(path, "base64:second"))
	content, _ = os.ReadFile(path)
	assert.Equal(t, "APP_ENV=local\nAPP_KEY=base64:second\nDB=x\n", string(content))

	// Appends when no APP_KEY exists (adding the missing trailing newline).
	require.NoError(t, os.WriteFile(path, []byte("APP_ENV=local"), 0o600))
	require.NoError(t, writeAppKey(path, "base64:third"))
	content, _ = os.ReadFile(path)
	assert.Equal(t, "APP_ENV=local\nAPP_KEY=base64:third\n", string(content))
}

func TestKeyGenerateCommandWritesEnv(t *testing.T) {
	app := testutil.NewMockApplication()
	envPath := filepath.Join(t.TempDir(), ".env")

	cmd := KeyGenerateCommand(app)
	cmd.SetArgs([]string{"--env", envPath})
	require.NoError(t, cmd.Execute())

	content, err := os.ReadFile(envPath)
	require.NoError(t, err)
	assert.Contains(t, string(content), "APP_KEY=base64:")
}

func TestMakeGeneratorCommands(t *testing.T) {
	app := testutil.NewMockApplication()
	base := t.TempDir()
	app.SetBasePath(base)

	cases := []struct {
		cmd      func() error
		expected string
		contains string
	}{
		{runGen(MakeJobCommand(app), "SendEmail"), "app/jobs/send_email_job.go", "type SendEmailJob struct"},
		{runGen(MakeEventCommand(app), "OrderShipped"), "app/events/order_shipped.go", "type OrderShipped struct"},
		{runGen(MakeListenerCommand(app), "SendReceipt"), "app/listeners/send_receipt.go", "func SendReceipt"},
		{runGen(MakeSeederCommand(app), "Users"), "database/seeders/users_seeder.go", "func UsersSeeder"},
		{runGen(MakeRequestCommand(app), "StoreUser"), "app/requests/store_user_request.go", "type StoreUserRequest struct"},
		{runGen(MakeCommandCommand(app), "SyncOrders"), "app/console/sync_orders.go", "func SyncOrdersCommand"},
		{runGen(MakePolicyCommand(app), "Post"), "app/policies/post.go", "func RegisterPostPolicy"},
	}

	for _, tc := range cases {
		require.NoError(t, tc.cmd())
		content, err := os.ReadFile(filepath.Join(base, filepath.FromSlash(tc.expected)))
		require.NoError(t, err, "expected %s to exist", tc.expected)
		assert.Contains(t, string(content), tc.contains)
	}

	// Generating the same file twice fails instead of overwriting.
	assert.ErrorContains(t, runGen(MakeJobCommand(app), "SendEmail")(), "already exists")
}

func runGen(cmd interface{ SetArgs([]string) }, name string) func() error {
	type executable interface {
		SetArgs([]string)
		Execute() error
	}
	e := cmd.(executable)
	return func() error {
		e.SetArgs([]string{name})
		return e.Execute()
	}
}

// --- migrate:reset / migrate:fresh over a real sqlite migrator ---

type widgetsMigration struct{}

func (m *widgetsMigration) Name() string { return "2026_01_01_000000_create_widgets" }
func (m *widgetsMigration) Up(b *schema.Builder) error {
	return b.Create("widgets", func(t *schema.Blueprint) {
		t.ID()
		t.String("name", 255)
	})
}
func (m *widgetsMigration) Down(b *schema.Builder) error { return b.Drop("widgets") }

func migratorApp(t *testing.T) (*testutil.MockApplication, *sql.DB) {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	migrator := migrations.NewMigrator(db, "sqlite", []migrations.Migration{&widgetsMigration{}}, nil)
	app := testutil.NewMockApplication()
	require.NoError(t, app.InstanceType(migrator))
	return app, db
}

func widgetTableExists(t *testing.T, db *sql.DB) bool {
	t.Helper()
	var count int
	require.NoError(t, db.QueryRow(
		"SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='widgets'").Scan(&count))
	return count > 0
}

func TestMigrateFreshAndResetCommands(t *testing.T) {
	app, db := migratorApp(t)

	require.NoError(t, MigrateFreshCommand(app).Execute())
	assert.True(t, widgetTableExists(t, db))

	require.NoError(t, MigrateResetCommand(app).Execute())
	assert.False(t, widgetTableExists(t, db))

	// Fresh from a reset state re-creates everything.
	require.NoError(t, MigrateFreshCommand(app).Execute())
	assert.True(t, widgetTableExists(t, db))
}

// --- db:seed ---

func TestDbSeedCommand(t *testing.T) {
	var ran []string
	runner := seed.NewRunner()
	runner.AddFunc("users", func() error { ran = append(ran, "users"); return nil })
	runner.AddFunc("posts", func() error { ran = append(ran, "posts"); return nil })

	app := testutil.NewMockApplication()
	require.NoError(t, app.InstanceType(runner))

	require.NoError(t, DbSeedCommand(app).Execute())
	assert.Equal(t, []string{"users", "posts"}, ran)

	ran = nil
	cmd := DbSeedCommand(app)
	cmd.SetArgs([]string{"--seeder", "posts"})
	require.NoError(t, cmd.Execute())
	assert.Equal(t, []string{"posts"}, ran)
}

// --- queue commands over the memory driver ---

type noopJob struct{}

func (j *noopJob) Handle() error { return nil }

func TestQueueCommands(t *testing.T) {
	queue.Register[noopJob]()

	manager := queue.NewManager()
	memory := queue.NewMemoryQueue()
	manager.Register("memory", memory)
	manager.SetDefaultConnection("memory")

	app := testutil.NewMockApplication()
	require.NoError(t, app.InstanceType(manager))

	// Work through the queued job with --stop-when-empty.
	require.NoError(t, memory.Push(&noopJob{}))
	work := QueueWorkCommand(app)
	work.SetArgs([]string{"--stop-when-empty"})
	require.NoError(t, work.Execute())
	assert.Equal(t, 0, memory.Size(""))

	// queue:failed on an empty failed list succeeds.
	require.NoError(t, QueueFailedCommand(app).Execute())

	// queue:retry rejects garbage ids.
	retry := QueueRetryCommand(app)
	retry.SetArgs([]string{"not-a-number"})
	assert.ErrorContains(t, retry.Execute(), "invalid job id")

	// The sync driver cannot run workers.
	manager.SetDefaultConnection("sync")
	work = QueueWorkCommand(app)
	work.SetArgs([]string{"--once"})
	err := work.Execute()
	assert.ErrorContains(t, err, "does not support workers")
}

// --- schedule commands ---

func TestScheduleCommands(t *testing.T) {
	ran := false
	scheduler := schedule.New()
	scheduler.Call(func() error { ran = true; return nil }).EveryMinute().Description("tick")

	app := testutil.NewMockApplication()
	require.NoError(t, app.InstanceType(scheduler))

	require.NoError(t, ScheduleListCommand(app).Execute())
	require.NoError(t, ScheduleRunCommand(app).Execute())
	assert.True(t, ran, "an every-minute task is always due")
}

// --- about ---

func TestAboutCommand(t *testing.T) {
	app := testutil.NewMockApplication()
	require.NoError(t, AboutCommand(app).Execute())
}

// Guard against generator paths escaping the base directory.
func TestGeneratorNamesAreSanitised(t *testing.T) {
	app := testutil.NewMockApplication()
	base := t.TempDir()
	app.SetBasePath(base)

	cmd := MakeJobCommand(app)
	cmd.SetArgs([]string{"weird name-here"})
	require.NoError(t, cmd.Execute())

	// PascalCase + snake_case sanitisation keeps the file inside app/jobs.
	entries, err := os.ReadDir(filepath.Join(base, "app", "jobs"))
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.False(t, strings.Contains(entries[0].Name(), " "))
}

func TestQueueWorkPriorityListIsTrimmed(t *testing.T) {
	queue.Register[noopJob]()

	manager := queue.NewManager()
	memory := queue.NewMemoryQueue()
	manager.Register("memory", memory)
	manager.SetDefaultConnection("memory")

	app := testutil.NewMockApplication()
	require.NoError(t, app.InstanceType(manager))

	// Jobs on both queues; a sloppy comma list with spaces must still
	// drain both (untrimmed " default" would poll a nonexistent queue).
	require.NoError(t, queue.PushOn(memory, "high", &noopJob{}))
	require.NoError(t, queue.PushOn(memory, "default", &noopJob{}))

	work := QueueWorkCommand(app)
	work.SetArgs([]string{"--queue", "high, default", "--stop-when-empty"})
	require.NoError(t, work.Execute())

	assert.Equal(t, 0, memory.Size("high"))
	assert.Equal(t, 0, memory.Size("default"))
}
