package providers

import (
	"github.com/genesysflow/go-genesys/contracts"
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
