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
	migrator := migrations.NewMigrator(conn.DB(), conn.Driver(), p.Migrations, p.BeforeAllMigrations)
	app.InstanceType(migrator)
	return app.BindValue("migrator", migrator)
}

// Provides returns the services this provider registers.
func (p *MigrationServiceProvider) Provides() []string {
	return []string{"migrator"}
}
