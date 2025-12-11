package providers

import (
	"github.com/genesysflow/go-genesys/contracts"
	"github.com/genesysflow/go-genesys/env"
	"github.com/genesysflow/go-genesys/errors"
)

// AppServiceProvider registers core application services.
type AppServiceProvider struct {
	BaseProvider
}

// Register registers the application services.
func (p *AppServiceProvider) Register(app contracts.Application) error {
	p.app = app

	// Register environment helper
	app.BindValue("env", env.NewHelper())

	// Register error handler
	logger := app.GetLogger()
	handler := errors.NewHandler(errors.Config{
		Debug:  app.IsDebug(),
		Logger: logger,
	})
	app.BindValue("error.handler", handler)

	return nil
}

// Boot bootstraps the application services.
func (p *AppServiceProvider) Boot(app contracts.Application) error {
	return nil
}

// Provides returns the services this provider registers.
func (p *AppServiceProvider) Provides() []string {
	return []string{
		"config",
		"env",
		"error.handler",
	}
}
