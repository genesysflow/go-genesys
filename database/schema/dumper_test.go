package schema

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/genesysflow/go-genesys/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	_ "modernc.org/sqlite"
)

// newDumperTestDB returns an in-memory SQLite database with a small schema.
// The dumper reads the catalog rather than compiling SQL, so it needs a real
// database; SQLite in memory keeps that cheap.
func newDumperTestDB(t *testing.T) *sql.DB {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	for _, stmt := range []string{
		`CREATE TABLE users (id INTEGER PRIMARY KEY AUTOINCREMENT, email VARCHAR(255) NOT NULL)`,
		`CREATE TABLE posts (id INTEGER PRIMARY KEY, user_id INTEGER NOT NULL, title TEXT)`,
		`CREATE INDEX posts_user_id_index ON posts (user_id)`,
		// Excluded from the dump: migration bookkeeping is not schema.
		`CREATE TABLE migrations (id INTEGER PRIMARY KEY, name TEXT)`,
	} {
		_, err := db.Exec(stmt)
		require.NoError(t, err, stmt)
	}

	// Force sqlite_sequence into existence so the exclusion is actually tested.
	_, err = db.Exec(`INSERT INTO users (email) VALUES ('a@example.com')`)
	require.NoError(t, err)

	return db
}

func TestNewDumper(t *testing.T) {
	db := newDumperTestDB(t)

	d := NewDumper(db, "sqlite")
	require.NotNil(t, d)
	assert.Equal(t, "sqlite", d.driver)
	assert.Same(t, db, d.db)
}

func TestDumperDumpSQLite(t *testing.T) {
	db := newDumperTestDB(t)
	path := filepath.Join(t.TempDir(), "schema.sql")

	require.NoError(t, NewDumper(db, "sqlite").Dump(path))

	dumped, err := os.ReadFile(path)
	require.NoError(t, err)
	out := string(dumped)

	assert.True(t, strings.HasPrefix(out, "-- Auto-generated schema dump\n"))
	assert.Contains(t, out, "CREATE TABLE posts")
	assert.Contains(t, out, "CREATE TABLE users")
	assert.Contains(t, out, "CREATE INDEX posts_user_id_index ON posts (user_id);")

	// The dump is meant to be replayable, so every statement is terminated.
	assert.Equal(t, strings.Count(out, "CREATE "), strings.Count(out, ";"),
		"each CREATE must be terminated exactly once")

	// Bookkeeping and SQLite internals are not part of the application schema.
	assert.NotContains(t, out, "migrations")
	assert.NotContains(t, out, "sqlite_sequence")

	// Statements come back ordered by name so the file is diffable between runs.
	assert.Less(t, strings.Index(out, "CREATE TABLE posts"), strings.Index(out, "CREATE TABLE users"))
}

func TestDumperDumpSQLiteIsDeterministic(t *testing.T) {
	db := newDumperTestDB(t)
	dir := t.TempDir()

	first := filepath.Join(dir, "a.sql")
	second := filepath.Join(dir, "b.sql")
	require.NoError(t, NewDumper(db, "sqlite").Dump(first))
	require.NoError(t, NewDumper(db, "sqlite3").Dump(second))

	a, err := os.ReadFile(first)
	require.NoError(t, err)
	b, err := os.ReadFile(second)
	require.NoError(t, err)
	assert.Equal(t, string(a), string(b))
}

func TestDumperDumpEmptySchema(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	path := filepath.Join(t.TempDir(), "schema.sql")
	require.NoError(t, NewDumper(db, "sqlite").Dump(path))

	dumped, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "-- Auto-generated schema dump\n", string(dumped))
}

// MySQL has no dump implementation, so it is refused rather than silently
// writing an empty file over an existing schema.
func TestDumperDumpUnsupportedDriver(t *testing.T) {
	db := newDumperTestDB(t)
	path := filepath.Join(t.TempDir(), "schema.sql")

	for _, driver := range []string{"mysql", "mariadb", "", "oracle"} {
		err := NewDumper(db, driver).Dump(path)
		require.Error(t, err, "driver %q", driver)
		assert.Contains(t, err.Error(), "not supported for schema dumping")
		assert.NoFileExists(t, path, "a refused dump must not write a file")
	}
}

func TestDumperDumpReportsQueryFailure(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	require.NoError(t, db.Close())

	err = NewDumper(db, "sqlite").Dump(filepath.Join(t.TempDir(), "schema.sql"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to query sqlite_master")
}

func TestDumperDumpReportsWriteFailure(t *testing.T) {
	db := newDumperTestDB(t)

	// A directory is not a writable file, so os.WriteFile must fail.
	err := NewDumper(db, "sqlite").Dump(t.TempDir())
	require.Error(t, err)
}

func TestDumperDumpPostgres(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	pc, cleanup := testutil.SetupPostgresContainer(t)
	defer cleanup()

	manager := newTestDatabaseManager(pc)
	defer manager.Close()

	db := manager.Connection().DB()

	for _, stmt := range []string{
		`CREATE TABLE teams (id SERIAL PRIMARY KEY, name VARCHAR(100) NOT NULL)`,
		`CREATE TABLE memberships (team_id INTEGER NOT NULL, user_id INTEGER NOT NULL,
			role VARCHAR(20) NOT NULL DEFAULT 'member', joined_at TIMESTAMP,
			PRIMARY KEY (team_id, user_id))`,
		`CREATE TABLE migrations (id SERIAL PRIMARY KEY, name TEXT)`,
	} {
		_, err := db.Exec(stmt)
		require.NoError(t, err, stmt)
	}

	path := filepath.Join(t.TempDir(), "schema.sql")
	require.NoError(t, NewDumper(db, "postgres").Dump(path))

	dumped, err := os.ReadFile(path)
	require.NoError(t, err)
	out := string(dumped)

	assert.True(t, strings.HasPrefix(out, "-- Auto-generated schema dump\n"))

	// A single-column primary key is inlined on the column.
	assert.Contains(t, out, `CREATE TABLE "teams" (`)
	assert.Contains(t, out, `"id" integer NOT NULL DEFAULT nextval('teams_id_seq'::regclass) PRIMARY KEY`)

	// character varying carries its length through.
	assert.Contains(t, out, `"name" character varying(100) NOT NULL`)

	// A composite primary key becomes a table-level clause instead.
	assert.Contains(t, out, `CREATE TABLE "memberships" (`)
	assert.Contains(t, out, "PRIMARY KEY (")
	assert.Contains(t, out, `"role" character varying(20) NOT NULL DEFAULT 'member'::character varying`)

	// A nullable column with no default carries neither clause.
	assert.Contains(t, out, `"joined_at" timestamp without time zone`)
	assert.NotContains(t, out, `"joined_at" timestamp without time zone NOT NULL`)

	assert.NotContains(t, out, `CREATE TABLE "migrations"`)

	// Tables are ordered by name so the file is diffable between runs.
	assert.Less(t, strings.Index(out, `CREATE TABLE "memberships"`), strings.Index(out, `CREATE TABLE "teams"`))

	// Every statement is terminated.
	assert.Equal(t, strings.Count(out, "CREATE TABLE"), strings.Count(out, ");"))
}

func TestDumperDumpPostgresReportsQueryFailure(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	require.NoError(t, db.Close())

	err = NewDumper(db, "pgsql").Dump(filepath.Join(t.TempDir(), "schema.sql"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to query information_schema.tables")
}
