package schema

import (
	"database/sql"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	_ "modernc.org/sqlite"
)

// Builder is otherwise only exercised against a live Postgres container, which
// covers the happy paths. These tests use in-memory SQLite to reach the failure
// paths cheaply — in particular that a failed ALTER batch leaves the table as it
// was, which is the reason Table() wraps the batch in a transaction at all.

func newSchemaTestDB(t *testing.T) *sql.DB {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	// A single connection: ":memory:" gives each pooled connection its own
	// database, so anything else makes the tests non-deterministic.
	db.SetMaxOpenConns(1)

	return db
}

func columnExists(t *testing.T, db *sql.DB, table, column string) bool {
	t.Helper()

	rows, err := db.Query(`SELECT name FROM pragma_table_info(?)`, table)
	require.NoError(t, err)
	defer rows.Close()

	for rows.Next() {
		var name string
		require.NoError(t, rows.Scan(&name))
		if name == column {
			return true
		}
	}
	require.NoError(t, rows.Err())
	return false
}

func TestBuilderCreateOnSQLite(t *testing.T) {
	db := newSchemaTestDB(t)
	b := NewBuilder(db, "sqlite")

	require.NoError(t, b.Create("users", func(bp *Blueprint) {
		bp.ID()
		bp.String("email", 255).Unique()
		bp.String("name", 100).Index()
		bp.Timestamps()
	}))

	assert.True(t, b.HasTable("users"))
	assert.True(t, columnExists(t, db, "users", "email"))

	// The declared index is created as a second statement, under the name the
	// drop path reconstructs.
	var count int
	require.NoError(t, db.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name='users_name_index'`).Scan(&count))
	assert.Equal(t, 1, count)
}

func TestBuilderCreateReportsCreateTableFailure(t *testing.T) {
	db := newSchemaTestDB(t)
	b := NewBuilder(db, "sqlite")

	require.NoError(t, b.Create("users", func(bp *Blueprint) { bp.ID() }))

	// The second Create emits the same unconditional CREATE TABLE.
	err := b.Create("users", func(bp *Blueprint) { bp.ID() })
	require.Error(t, err)
	assert.Contains(t, err.Error(), "users")
}

// The table is created before its indexes, so an index failure leaves the table
// behind. The error is surfaced rather than swallowed.
func TestBuilderCreateReportsIndexFailure(t *testing.T) {
	db := newSchemaTestDB(t)
	b := NewBuilder(db, "sqlite")

	// Index names are derived from the columns, so declaring the same index
	// twice emits the same CREATE INDEX twice and the second one collides.
	err := b.Create("users", func(bp *Blueprint) {
		bp.Integer("tenant_id")
		bp.Index("tenant_id")
		bp.Index("tenant_id")
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "users_tenant_id_index")

	assert.True(t, b.HasTable("users"), "the CREATE TABLE had already committed")
}

func TestBuilderTableOnSQLite(t *testing.T) {
	db := newSchemaTestDB(t)
	b := NewBuilder(db, "sqlite")

	require.NoError(t, b.Create("users", func(bp *Blueprint) {
		bp.ID()
		bp.String("email_address", 255)
		bp.String("legacy", 50).Nullable()
	}))

	require.NoError(t, b.Table("users", func(bp *Blueprint) {
		bp.AddString("phone", 20).Nullable()
		bp.AddBoolean("verified").Default(false)
		bp.RenameColumn("email_address", "email")
		bp.DropColumn("legacy")
	}))

	assert.True(t, columnExists(t, db, "users", "phone"))
	assert.True(t, columnExists(t, db, "users", "verified"))
	assert.True(t, columnExists(t, db, "users", "email"))
	assert.False(t, columnExists(t, db, "users", "email_address"))
	assert.False(t, columnExists(t, db, "users", "legacy"))
}

// Table() runs its statements in a transaction so a migration is all-or-nothing.
// Without the rollback the first ADD COLUMN would survive and a re-run would
// then fail on the duplicate column instead.
func TestBuilderTableRollsBackTheWholeBatchOnFailure(t *testing.T) {
	db := newSchemaTestDB(t)
	b := NewBuilder(db, "sqlite")

	require.NoError(t, b.Create("users", func(bp *Blueprint) { bp.ID() }))

	err := b.Table("users", func(bp *Blueprint) {
		bp.AddString("phone", 20).Nullable()
		bp.DropColumn("no_such_column")
	})
	require.Error(t, err)

	assert.False(t, columnExists(t, db, "users", "phone"),
		"the successful statement must be rolled back with the failed one")
}

func TestBuilderTableReportsBeginFailure(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	require.NoError(t, db.Close())

	err = NewBuilder(db, "sqlite").Table("users", func(bp *Blueprint) {
		bp.AddString("phone", 20)
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "closed")
}

func TestBuilderDropAndRenameOnSQLite(t *testing.T) {
	db := newSchemaTestDB(t)
	b := NewBuilder(db, "sqlite")

	require.NoError(t, b.Create("users", func(bp *Blueprint) { bp.ID() }))

	require.NoError(t, b.Rename("users", "people"))
	assert.False(t, b.HasTable("users"))
	assert.True(t, b.HasTable("people"))

	require.NoError(t, b.Drop("people"))
	assert.False(t, b.HasTable("people"))

	// Drop is unconditional; DropIfExists is the idempotent form.
	require.Error(t, b.Drop("people"))
	require.NoError(t, b.DropIfExists("people"))
}

// HasTable answers false rather than propagating an error, so a caller that
// cannot reach the database sees "no such table".
func TestBuilderHasTableIsFalseWhenTheQueryFails(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	require.NoError(t, db.Close())

	assert.False(t, NewBuilder(db, "sqlite").HasTable("users"))
}
