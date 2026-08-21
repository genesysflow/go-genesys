package console

import (
	"github.com/genesysflow/go-genesys/console/commands"
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
	p.kernel.AddCommand(commands.DbSchemaDumpCommand(app))
	p.kernel.AddCommand(commands.MakeControllerCommand(app))
	p.kernel.AddCommand(commands.MakeModelCommand(app))
	p.kernel.AddCommand(commands.MakeMiddlewareCommand(app))
	p.kernel.AddCommand(commands.MakeProviderCommand(app))
	p.kernel.AddCommand(commands.SqlcGenerateCommand(app))
	p.kernel.AddCommand(commands.KeyGenerateCommand(app))
	p.kernel.AddCommand(commands.QueueWorkCommand(app))
	p.kernel.AddCommand(commands.QueueFailedCommand(app))
	p.kernel.AddCommand(commands.QueuePruneFailedCommand(app))
	p.kernel.AddCommand(commands.QueueRetryCommand(app))
	p.kernel.AddCommand(commands.ScheduleRunCommand(app))
	p.kernel.AddCommand(commands.ScheduleWorkCommand(app))
	p.kernel.AddCommand(commands.ScheduleListCommand(app))
	p.kernel.AddCommand(commands.DbSeedCommand(app))
	p.kernel.AddCommand(commands.MigrateResetCommand(app))
	p.kernel.AddCommand(commands.MigrateFreshCommand(app))
	p.kernel.AddCommand(commands.RouteListCommand(app))
	p.kernel.AddCommand(commands.AboutCommand(app))
	p.kernel.AddCommand(commands.MakeJobCommand(app))
	p.kernel.AddCommand(commands.MakeEventCommand(app))
	p.kernel.AddCommand(commands.MakeListenerCommand(app))
	p.kernel.AddCommand(commands.MakeSeederCommand(app))
	p.kernel.AddCommand(commands.MakeRequestCommand(app))
	p.kernel.AddCommand(commands.MakeCommandCommand(app))
	p.kernel.AddCommand(commands.MakePolicyCommand(app))
	p.kernel.AddCommand(commands.DownCommand(app))
	p.kernel.AddCommand(commands.UpCommand(app))
	p.kernel.AddCommand(commands.CacheClearCommand(app).Cobra(app))
	p.kernel.AddCommand(commands.CacheForgetCommand(app).Cobra(app))
	p.kernel.AddCommand(commands.StorageLinkCommand(app).Cobra(app))
	p.kernel.AddCommand(commands.ConfigShowCommand(app).Cobra(app))
	p.kernel.AddCommand(commands.MigrateRefreshCommand(app).Cobra(app))
	p.kernel.AddCommand(commands.DbWipeCommand(app).Cobra(app))
	p.kernel.AddCommand(commands.DbShowCommand(app).Cobra(app))
	p.kernel.AddCommand(commands.DbTableCommand(app).Cobra(app))
	p.kernel.AddCommand(commands.ScheduleTestCommand(app).Cobra(app))
	p.kernel.AddCommand(commands.EventListCommand(app).Cobra(app))
	p.kernel.AddCommand(commands.MakeMailCommand(app).Cobra(app))
	p.kernel.AddCommand(commands.MakeNotificationCommand(app).Cobra(app))
	p.kernel.AddCommand(commands.MakeFactoryCommand(app).Cobra(app))
	p.kernel.AddCommand(commands.MakeResourceCommand(app).Cobra(app))
	p.kernel.AddCommand(commands.MakeRuleCommand(app).Cobra(app))
	p.kernel.AddCommand(commands.MakeObserverCommand(app).Cobra(app))
	p.kernel.AddCommand(commands.MakeCastCommand(app).Cobra(app))
	p.kernel.AddCommand(commands.MakeScopeCommand(app).Cobra(app))
	p.kernel.AddCommand(commands.MakeChannelCommand(app).Cobra(app))
	p.kernel.AddCommand(commands.MakeExceptionCommand(app).Cobra(app))
	p.kernel.AddCommand(commands.MakeEnumCommand(app).Cobra(app))
	p.kernel.AddCommand(commands.MakeTestCommand(app).Cobra(app))
	p.kernel.AddCommand(commands.MakeViewCommand(app).Cobra(app))
	p.kernel.AddCommand(commands.MakeComponentCommand(app).Cobra(app))

	// Bind kernel to container
	app.InstanceType(p.kernel)
	app.BindValue("console.kernel", p.kernel)
	app.BindValue("console.kernel.interface", p.kernel)

	// Bind routes and middleware if provided
	if p.Routes != nil {
		app.InstanceType(p.Routes)
		app.BindValue("console.routes", p.Routes)
	}
	if p.Middleware != nil {
		app.InstanceType(p.Middleware)
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
