package providers

import (
	"fmt"

	"github.com/genesysflow/go-genesys/container"
	"github.com/genesysflow/go-genesys/contracts"
	"github.com/genesysflow/go-genesys/database"
	"github.com/genesysflow/go-genesys/database/migrations"
)

// MigrationServiceProvider registers the migrator service.
type MigrationServiceProvider struct {
	BaseProvider
	BeforeAllMigrations func() error
	Migrations          []migrations.Migration
}

// Register registers the migration services.
func (p *MigrationServiceProvider) Register(app contracts.Application) error {
	p.app = app
	return nil
}

// Boot bootstraps the migration services.
func (p *MigrationServiceProvider) Boot(app contracts.Application) error {
	mgr, err := container.Resolve[*database.Manager](app)
	if err != nil {
		return fmt.Errorf("failed to resolve db manager: %w", err)
	}

	conn := mgr.Connection()
	if conn == nil {
		return fmt.Errorf("no default database connection available")
	}

	// Check if connection was established successfully
	if conn.DB() == nil {
		// Provide more context if possible, maybe check if we can access the error
		// connection might be implementing an interface that hides the error field
		// but checking for nil DB is safe.
		// If contracts.Connection has Check/Ping, we could use that, but DB() check is direct.
		return fmt.Errorf("failed to establish database connection: default connection has nil DB")
	}

	migrator := migrations.NewMigrator(conn.DB(), conn.Driver(), p.Migrations, p.BeforeAllMigrations)
	app.InstanceType(migrator)
	return app.BindValue("migrator", migrator)
}

// Provides returns the services this provider registers.
func (p *MigrationServiceProvider) Provides() []string {
	return []string{"migrator"}
}
