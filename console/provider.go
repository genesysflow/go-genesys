package console

import (
	"reflect"

	"github.com/genesysflow/go-genesys/console/commands"
	"github.com/genesysflow/go-genesys/container"
	"github.com/genesysflow/go-genesys/contracts"
	"github.com/genesysflow/go-genesys/http"
	"github.com/spf13/cobra"
)

// ConsoleServiceProvider registers console services and commands.
type ConsoleServiceProvider struct {
	// AppName is the application name used in the CLI.
	AppName string

	// AppShort is a short description of the application.
	AppShort string

	// AppLong is a long description of the application.
	AppLong string

	// Routes is an optional function that defines HTTP routes for the serve command.
	Routes func(*http.Router)

	// Middleware is optional global middleware for the serve command.
	Middleware []http.MiddlewareFunc

	// Commands is an optional function that registers custom commands.
	// This callback is executed after framework commands are registered.
	Commands func(*cobra.Command)

	app    contracts.Application
	kernel *Kernel
}

// Register registers the console services.
func (p *ConsoleServiceProvider) Register(app contracts.Application) error {
	p.app = app

	// Set defaults
	if p.AppName == "" {
		p.AppName = "app"
	}
	if p.AppShort == "" {
		p.AppShort = p.AppName + " - A Go-Genesys application"
	}
	if p.AppLong == "" {
		p.AppLong = p.AppName + " is a web application built with the Go-Genesys framework."
	}

	// Create console kernel
	config := KernelConfig{
		Name:  p.AppName,
		Short: p.AppShort,
		Long:  p.AppLong,
	}
	p.kernel = NewKernel(app, config)

	// Register framework commands
	p.kernel.AddCommand(commands.ServeCommand(app))
	p.kernel.AddCommand(commands.MigrateCommand(app))
	p.kernel.AddCommand(commands.MigrateRollbackCommand(app))
	p.kernel.AddCommand(commands.MigrateStatusCommand(app))
	p.kernel.AddCommand(commands.MakeMigrationCommand(app))
	p.kernel.AddCommand(commands.MakeControllerCommand(app))
	p.kernel.AddCommand(commands.MakeModelCommand(app))
	p.kernel.AddCommand(commands.MakeMiddlewareCommand(app))
	p.kernel.AddCommand(commands.MakeProviderCommand(app))

	// Bind kernel to container
	appValue := reflect.ValueOf(app)
	if appValue.Kind() == reflect.Ptr {
		appValue = appValue.Elem()
	}

	containerField := appValue.FieldByName("Container")
	if containerField.IsValid() && !containerField.IsNil() {
		containerPtr := containerField.Interface().(*container.Container)
		container.ProvideNamedValue[*Kernel](containerPtr, "console.kernel", p.kernel)
		container.ProvideNamedValue[contracts.Kernel](containerPtr, "console.kernel.interface", p.kernel)
	} else {
		app.BindValue("console.kernel", p.kernel)
		app.BindValue("console.kernel.interface", p.kernel)
	}

	// Bind routes and middleware if provided
	if p.Routes != nil {
		app.BindValue("console.routes", p.Routes)
	}
	if p.Middleware != nil {
		app.BindValue("console.middleware", p.Middleware)
	}

	return nil
}

// Boot bootstraps the console services.
func (p *ConsoleServiceProvider) Boot(app contracts.Application) error {
	// Register custom commands if provided
	if p.Commands != nil && p.kernel != nil {
		p.Commands(p.kernel.RootCommand())
	}

	return nil
}

// Provides returns the services this provider registers.
func (p *ConsoleServiceProvider) Provides() []string {
	return []string{
		"console.kernel",
		"console.kernel.interface",
	}
}

// Kernel returns the console kernel.
func (p *ConsoleServiceProvider) Kernel() *Kernel {
	return p.kernel
}
