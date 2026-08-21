package providers

import (
	"fmt"

	"github.com/genesysflow/go-genesys/container"
	"github.com/genesysflow/go-genesys/contracts"
	"github.com/genesysflow/go-genesys/database"
	"github.com/genesysflow/go-genesys/query"
	"github.com/genesysflow/go-genesys/validation"
)

// ValidationServiceProvider registers validation services.
type ValidationServiceProvider struct {
	BaseProvider

	// CustomMessages are custom validation error messages.
	CustomMessages map[string]string

	// AttributeNames are custom attribute names for error messages.
	AttributeNames map[string]string
}

// Register registers the validation services.
func (p *ValidationServiceProvider) Register(app contracts.Application) error {
	p.app = app

	v := validation.New()

	// Set custom messages if provided
	if len(p.CustomMessages) > 0 {
		v.SetMessages(p.CustomMessages)
	}

	// Set attribute names if provided
	if len(p.AttributeNames) > 0 {
		v.SetAttributeNames(p.AttributeNames)
	}

	// The database-backed rules (unique, exists) resolve their connection
	// when they run, not now: the database provider may be registered
	// after this one, and a reconnect must be picked up.
	v.SetDatabaseResolver(func() (string, query.Executor, error) {
		manager, err := container.Resolve[*database.Manager](app)
		if err != nil {
			return "", nil, fmt.Errorf("validation: no database configured for unique/exists rules: %w", err)
		}

		conn := manager.Connection()
		if conn == nil {
			return "", nil, fmt.Errorf("validation: no database connection for unique/exists rules")
		}

		return conn.Driver(), conn, nil
	})

	app.InstanceType(v)
	app.BindValue("validator", v)

	return nil
}

// Boot bootstraps the validation services.
func (p *ValidationServiceProvider) Boot(app contracts.Application) error {
	return nil
}

// Provides returns the services this provider registers.
func (p *ValidationServiceProvider) Provides() []string {
	return []string{
		"validator",
	}
}
