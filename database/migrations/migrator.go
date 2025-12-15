// Package migrations provides database migration functionality.
package migrations

import (
	"database/sql"
	"fmt"
	"sort"
	"time"

	"github.com/genesysflow/go-genesys/database/schema"
)

// Migration represents a database migration.
type Migration interface {
	// Up runs the migration.
	Up(builder *schema.Builder) error

	// Down reverses the migration.
	Down(builder *schema.Builder) error

	// Name returns the migration name.
	Name() string
}

// Migrator handles running migrations.
type Migrator struct {
	db                  *sql.DB
	driver              string
	table               string
	migrations          []Migration
	beforeAllMigrations func() error
}

// NewMigrator creates a new migrator.
func NewMigrator(db *sql.DB, driver string, migrations []Migration, beforeAllMigrations func() error) *Migrator {
	return &Migrator{
		db:                  db,
		driver:              driver,
		table:               "migrations",
		migrations:          migrations,
		beforeAllMigrations: beforeAllMigrations,
	}
}

// SetTable sets the migrations table name.
func (m *Migrator) SetTable(table string) {
	m.table = table
}

// Register registers a migration.
func (m *Migrator) Register(migration Migration) {
	m.migrations = append(m.migrations, migration)
}

// RegisterAll registers multiple migrations.
func (m *Migrator) RegisterAll(migrations []Migration) {
	m.migrations = append(m.migrations, migrations...)
}

// createMigrationsTable creates the migrations table if it doesn't exist.
func (m *Migrator) createMigrationsTable() error {
	query := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			migration VARCHAR(255) NOT NULL,
			batch INTEGER NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`, m.table)

	_, err := m.db.Exec(query)
	return err
}

// getRanMigrations returns the list of migrations that have been run.
func (m *Migrator) getRanMigrations() (map[string]int, error) {
	if err := m.createMigrationsTable(); err != nil {
		return nil, err
	}

	query := fmt.Sprintf("SELECT migration, batch FROM %s", m.table)
	rows, err := m.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	ran := make(map[string]int)
	for rows.Next() {
		var name string
		var batch int
		if err := rows.Scan(&name, &batch); err != nil {
			return nil, err
		}
		ran[name] = batch
	}

	return ran, rows.Err()
}

// getLastBatch returns the last batch number.
func (m *Migrator) getLastBatch() (int, error) {
	query := fmt.Sprintf("SELECT COALESCE(MAX(batch), 0) FROM %s", m.table)
	var batch int
	err := m.db.QueryRow(query).Scan(&batch)
	return batch, err
}

// getMigrationsForBatch returns migrations for a specific batch.
func (m *Migrator) getMigrationsForBatch(batch int) ([]string, error) {
	query := fmt.Sprintf("SELECT migration FROM %s WHERE batch = ? ORDER BY id DESC", m.table)
	rows, err := m.db.Query(query, batch)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var migrations []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		migrations = append(migrations, name)
	}

	return migrations, rows.Err()
}

// Run runs all pending migrations.
func (m *Migrator) Run() ([]string, error) {
	if m.beforeAllMigrations != nil {
		if err := m.beforeAllMigrations(); err != nil {
			return nil, fmt.Errorf("before all migrations failed: %w", err)
		}
	}
	ran, err := m.getRanMigrations()
	if err != nil {
		return nil, fmt.Errorf("failed to get ran migrations: %w", err)
	}

	batch, err := m.getLastBatch()
	if err != nil {
		return nil, fmt.Errorf("failed to get last batch: %w", err)
	}
	batch++

	// Sort migrations by name
	sort.Slice(m.migrations, func(i, j int) bool {
		return m.migrations[i].Name() < m.migrations[j].Name()
	})

	var runNames []string
	for _, migration := range m.migrations {
		name := migration.Name()
		if _, ok := ran[name]; ok {
			continue // Already run
		}
		builder := schema.NewBuilder(m.db, m.driver)

		if err := migration.Up(builder); err != nil {
			return runNames, fmt.Errorf("migration %s failed: %w", name, err)
		}

		// Record migration
		query := fmt.Sprintf("INSERT INTO %s (migration, batch) VALUES (?, ?)", m.table)
		if _, err := m.db.Exec(query, name, batch); err != nil {
			return runNames, fmt.Errorf("failed to record migration %s: %w", name, err)
		}

		runNames = append(runNames, name)
	}

	return runNames, nil
}

// Rollback rolls back the last batch of migrations.
func (m *Migrator) Rollback() ([]string, error) {
	batch, err := m.getLastBatch()
	if err != nil {
		return nil, fmt.Errorf("failed to get last batch: %w", err)
	}

	if batch == 0 {
		return nil, nil // Nothing to rollback
	}

	migrations, err := m.getMigrationsForBatch(batch)
	if err != nil {
		return nil, fmt.Errorf("failed to get batch migrations: %w", err)
	}

	// Create migration name to migration map
	migrationMap := make(map[string]Migration)
	for _, mig := range m.migrations {
		migrationMap[mig.Name()] = mig
	}

	var rolledBack []string
	for _, name := range migrations {
		migration, ok := migrationMap[name]
		if !ok {
			return rolledBack, fmt.Errorf("migration %s not found in registered migrations", name)
		}

		builder := schema.NewBuilder(m.db, m.driver)
		if err := migration.Down(builder); err != nil {
			return rolledBack, fmt.Errorf("rollback of %s failed: %w", name, err)
		}

		// Remove migration record
		query := fmt.Sprintf("DELETE FROM %s WHERE migration = ?", m.table)
		if _, err := m.db.Exec(query, name); err != nil {
			return rolledBack, fmt.Errorf("failed to remove migration record %s: %w", name, err)
		}

		rolledBack = append(rolledBack, name)
	}

	return rolledBack, nil
}

// Reset rolls back all migrations.
func (m *Migrator) Reset() ([]string, error) {
	var allRolledBack []string

	for {
		rolledBack, err := m.Rollback()
		if err != nil {
			return allRolledBack, err
		}
		if len(rolledBack) == 0 {
			break
		}
		allRolledBack = append(allRolledBack, rolledBack...)
	}

	return allRolledBack, nil
}

// Status returns the status of all migrations.
func (m *Migrator) Status() ([]MigrationStatus, error) {
	ran, err := m.getRanMigrations()
	if err != nil {
		return nil, err
	}

	var status []MigrationStatus
	for _, migration := range m.migrations {
		name := migration.Name()
		batch, hasRun := ran[name]
		status = append(status, MigrationStatus{
			Name:  name,
			Ran:   hasRun,
			Batch: batch,
		})
	}

	// Sort by name
	sort.Slice(status, func(i, j int) bool {
		return status[i].Name < status[j].Name
	})

	return status, nil
}

// MigrationStatus represents the status of a migration.
type MigrationStatus struct {
	Name  string
	Ran   bool
	Batch int
}

// BaseMigration provides a base implementation for migrations.
type BaseMigration struct {
	name string
	up   func(*sql.DB) error
	down func(*sql.DB) error
}

// NewMigration creates a new migration.
func NewMigration(name string, up, down func(*sql.DB) error) *BaseMigration {
	return &BaseMigration{
		name: name,
		up:   up,
		down: down,
	}
}

// Name returns the migration name.
func (m *BaseMigration) Name() string {
	return m.name
}

// Up runs the migration.
func (m *BaseMigration) Up(db *sql.DB) error {
	if m.up == nil {
		return nil
	}
	return m.up(db)
}

// Down reverses the migration.
func (m *BaseMigration) Down(db *sql.DB) error {
	if m.down == nil {
		return nil
	}
	return m.down(db)
}

// GenerateMigrationName generates a migration name with timestamp.
func GenerateMigrationName(name string) string {
	return fmt.Sprintf("%s_%s", time.Now().Format("2006_01_02_150405"), name)
}
