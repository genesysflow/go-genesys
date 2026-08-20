package migrations_test

import (
	"database/sql"
	"errors"
	"testing"

	"github.com/genesysflow/go-genesys/database/migrations"
	"github.com/genesysflow/go-genesys/database/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

type createAccountsTable struct{}

func (m *createAccountsTable) Name() string { return "2026_01_01_000000_create_accounts_table" }
func (m *createAccountsTable) Up(b *schema.Builder) error {
	return b.Create("accounts", func(t *schema.Blueprint) {
		t.ID()
		t.String("name", 255)
		t.Timestamps()
	})
}
func (m *createAccountsTable) Down(b *schema.Builder) error { return b.Drop("accounts") }

type createInvoicesTable struct{}

func (m *createInvoicesTable) Name() string { return "2026_01_02_000000_create_invoices_table" }
func (m *createInvoicesTable) Up(b *schema.Builder) error {
	return b.Create("invoices", func(t *schema.Blueprint) {
		t.ID()
		t.ForeignID("account_id")
		t.Foreign("account_id").CascadeOnDelete()
		t.Decimal("total", 10, 2)
	})
}
func (m *createInvoicesTable) Down(b *schema.Builder) error { return b.Drop("invoices") }

type failingMigration struct{}

func (m *failingMigration) Name() string                   { return "2026_01_03_000000_failing" }
func (m *failingMigration) Up(b *schema.Builder) error     { return errors.New("boom") }
func (m *failingMigration) Down(b *schema.Builder) error   { return nil }

func newSqliteMigrator(t *testing.T, migrations_ []migrations.Migration) (*migrations.Migrator, *sql.DB) {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	return migrations.NewMigrator(db, "sqlite", migrations_, nil), db
}

func tableExists(t *testing.T, db *sql.DB, name string) bool {
	t.Helper()
	var count int
	require.NoError(t, db.QueryRow(
		"SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name = ?", name).Scan(&count))
	return count > 0
}

func TestMigratorRunRollbackResetOnSqlite(t *testing.T) {
	migrator, db := newSqliteMigrator(t, []migrations.Migration{
		&createAccountsTable{},
		&createInvoicesTable{},
	})

	// Run applies both migrations in one batch.
	ran, err := migrator.Run()
	require.NoError(t, err)
	assert.Len(t, ran, 2)
	assert.True(t, tableExists(t, db, "accounts"))
	assert.True(t, tableExists(t, db, "invoices"))

	// A second run is a no-op.
	ran, err = migrator.Run()
	require.NoError(t, err)
	assert.Empty(t, ran)

	// Status reports both as ran.
	statuses, err := migrator.Status()
	require.NoError(t, err)
	require.Len(t, statuses, 2)
	for _, status := range statuses {
		assert.True(t, status.Ran, "%s should be marked as ran", status.Name)
	}

	// Rollback undoes the whole last batch.
	rolledBack, err := migrator.Rollback()
	require.NoError(t, err)
	assert.Len(t, rolledBack, 2)
	assert.False(t, tableExists(t, db, "accounts"))
	assert.False(t, tableExists(t, db, "invoices"))

	// Re-run, then Reset drops everything again.
	_, err = migrator.Run()
	require.NoError(t, err)
	reset, err := migrator.Reset()
	require.NoError(t, err)
	assert.Len(t, reset, 2)
	assert.False(t, tableExists(t, db, "accounts"))
}

func TestMigratorBatches(t *testing.T) {
	migrator, db := newSqliteMigrator(t, []migrations.Migration{&createAccountsTable{}})

	_, err := migrator.Run()
	require.NoError(t, err)

	// A migration registered later lands in a new batch...
	migrator.Register(&createInvoicesTable{})
	ran, err := migrator.Run()
	require.NoError(t, err)
	assert.Equal(t, []string{"2026_01_02_000000_create_invoices_table"}, ran)

	// ...so rollback only unwinds that batch.
	rolledBack, err := migrator.Rollback()
	require.NoError(t, err)
	assert.Equal(t, []string{"2026_01_02_000000_create_invoices_table"}, rolledBack)
	assert.True(t, tableExists(t, db, "accounts"))
	assert.False(t, tableExists(t, db, "invoices"))
}

func TestMigratorFailureStopsRun(t *testing.T) {
	migrator, db := newSqliteMigrator(t, []migrations.Migration{
		&createAccountsTable{},
		&failingMigration{},
	})

	_, err := migrator.Run()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "boom")

	// The successful migration before the failure is recorded.
	assert.True(t, tableExists(t, db, "accounts"))
	statuses, err := migrator.Status()
	require.NoError(t, err)
	ranCount := 0
	for _, status := range statuses {
		if status.Ran {
			ranCount++
		}
	}
	assert.Equal(t, 1, ranCount)
}
