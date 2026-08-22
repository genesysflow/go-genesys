package bootstrap

import (
	"time"

	"github.com/genesysflow/go-genesys/auth"
	"github.com/genesysflow/go-genesys/console"
	"github.com/genesysflow/go-genesys/database/migrations"
	"github.com/genesysflow/go-genesys/database/seed"
	appconsole "github.com/genesysflow/go-genesys/example/app/console"
	"github.com/genesysflow/go-genesys/example/app/models"
	appProviders "github.com/genesysflow/go-genesys/example/app/providers"
	"github.com/genesysflow/go-genesys/example/app/tasks"
	m "github.com/genesysflow/go-genesys/example/database/migrations"
	"github.com/genesysflow/go-genesys/example/database/seeders"
	"github.com/genesysflow/go-genesys/example/routes"
	"github.com/genesysflow/go-genesys/foundation"
	"github.com/genesysflow/go-genesys/http"
	"github.com/genesysflow/go-genesys/providers"
	"github.com/genesysflow/go-genesys/schedule"
	"github.com/spf13/cobra"
)

// App creates and configures the application instance.
//
// The base path defaults to the working directory, which is where the
// binary runs from. Tests pass the application root explicitly, since a
// package's tests run in their own directory.
func App(basePath ...string) *foundation.Application {
	root := "."
	if len(basePath) > 0 && basePath[0] != "" {
		root = basePath[0]
	}

	app := foundation.New(root)

	// Register core service providers
	app.Register(&providers.AppServiceProvider{})
	app.Register(&appProviders.AppServiceProvider{})
	app.Register(&providers.LogServiceProvider{})
	app.Register(&providers.ValidationServiceProvider{})
	app.Register(&providers.SessionServiceProvider{})
	app.Register(&providers.DatabaseServiceProvider{})
	app.Register(&providers.FilesystemServiceProvider{})
	app.Register(&providers.CacheServiceProvider{})
	app.Register(&providers.QueueServiceProvider{})
	app.Register(&providers.EventServiceProvider{})
	app.Register(&providers.EncryptionServiceProvider{})
	app.Register(&providers.ViewServiceProvider{})
	app.Register(&providers.MailServiceProvider{})
	app.Register(&providers.NotificationServiceProvider{})
	app.Register(&providers.LangServiceProvider{})

	// Authentication: the session guard for the site, and (once the
	// database is up) the token repository the API authenticates with.
	app.Register(&providers.AuthServiceProvider{
		UserProvider: auth.NewORMUserProvider[models.User](),
	})

	// The blog's own wiring: policies, the polymorphic registry, queued
	// jobs and event listeners.
	app.Register(&appProviders.BlogServiceProvider{})

	// Scheduled tasks: run with `example schedule:work` (or schedule:run
	// from system cron).
	app.Register(&providers.ScheduleServiceProvider{
		Define: func(s *schedule.Schedule) {
			s.Call(func() error { return nil }).Hourly().Description("example heartbeat")

			// Housekeeping: only in production, only at night, and only
			// on one instance when several run this schedule.
			s.Call(tasks.PruneStaleDrafts).
				DailyAt("03:00").
				Timezone("UTC").
				Environments("production").
				OnOneServer().
				Description("prune stale drafts")

			// A digest, dispatched to a worker rather than run inline.
			s.Call(tasks.QueueWeeklyDigest(app)).
				WeeklyOn(1, "08:00").
				Description("queue the weekly digest")
		},
	})

	// Database seeders: run with `example db:seed`.
	app.Register(&providers.SeedServiceProvider{
		Define: func(r *seed.Runner) {
			r.AddFunc("users", seeders.SeedUsers)
			r.AddFunc("blog", seeders.SeedBlog)
		},
	})

	app.Register(&providers.MigrationServiceProvider{
		BeforeAllMigrations: m.BeforeAllMigrations,
		Migrations: []migrations.Migration{
			&m.CreateUsersTable{},
			&m.CreateBlogTables{},
			&m.AddRoleToUsers{},
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
		Commands: func(root *cobra.Command) {
			root.AddCommand(appconsole.BlogStatsCommand(app).Cobra(app))
		},
	})

	return app
}
