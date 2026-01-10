package bootstrap

import (
	"time"

	"github.com/genesysflow/go-genesys/console"
	"github.com/genesysflow/go-genesys/database/migrations"
	appProviders "github.com/genesysflow/go-genesys/example/app/providers"
	m "github.com/genesysflow/go-genesys/example/database/migrations"
	"github.com/genesysflow/go-genesys/example/routes"
	"github.com/genesysflow/go-genesys/foundation"
	"github.com/genesysflow/go-genesys/http"
	"github.com/genesysflow/go-genesys/providers"
)

// App creates and configures the application instance.
func App() *foundation.Application {
	app := foundation.New(".")

	// Register core service providers
	app.Register(&providers.AppServiceProvider{})
	app.Register(&appProviders.AppServiceProvider{})
	app.Register(&providers.LogServiceProvider{})
	app.Register(&providers.ValidationServiceProvider{})
	app.Register(&providers.SessionServiceProvider{})
	app.Register(&providers.DatabaseServiceProvider{})
	app.Register(&providers.FilesystemServiceProvider{})

	app.Register(&providers.MigrationServiceProvider{
		BeforeAllMigrations: m.BeforeAllMigrations,
		Migrations: []migrations.Migration{
			&m.CreateUsersTable{},
			// DO NOT DELETE: Add new migrations here
		},
	})

	// Configure HTTP kernel for increased body limits (e.g., for PDF uploads)
	kernelConfig := &http.KernelConfig{
		AppName:           "Example App",
		ServerHeader:      "Example",
		BodyLimit:         100 * 1024 * 1024, // 100MB for large file uploads
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      60 * time.Second,
		EnablePrintRoutes: false,
	}
	app.InstanceType(kernelConfig)

	// Register console service provider
	app.Register(&console.ConsoleServiceProvider{
		AppName:    "example",
		AppShort:   "Go-Genesys Example Application",
		AppLong:    "A demonstration application showcasing the Go-Genesys framework features.",
		Routes:     routes.Register,
		Middleware: routes.GlobalMiddleware(app),
	})

	return app
}
