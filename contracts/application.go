package contracts

import "context"

// Application defines the interface for the main application container.
// It orchestrates the entire framework lifecycle.
type Application interface {
	Container

	// Version returns the framework version.
	Version() string

	// BasePath returns the base path of the application.
	BasePath() string

	// SetBasePath sets the base path of the application.
	SetBasePath(path string) Application

	// ConfigPath returns the path to the config directory.
	ConfigPath() string

	// StoragePath returns the path to the storage directory.
	StoragePath() string

	// Environment returns the current environment (e.g., "production", "local").
	Environment() string

	// IsEnvironment checks if the app is running in the given environment(s).
	IsEnvironment(envs ...string) bool

	// IsProduction checks if the app is running in production.
	IsProduction() bool

	// IsLocal checks if the app is running locally.
	IsLocal() bool

	// IsDebug checks if debug mode is enabled.
	IsDebug() bool

	// Register registers a service provider with the application.
	Register(provider ServiceProvider) error

	// Boot boots the application and all registered providers.
	Boot() error

	// IsBooted returns true if the application has been booted.
	IsBooted() bool

	// Booting registers a callback to be run before booting.
	Booting(callback func(Application))

	// Booted registers a callback to be run after booting.
	Booted(callback func(Application))

	// Terminating registers a callback to be run during termination.
	Terminating(callback func(Application))

	// Terminate terminates the application.
	Terminate() error

	// TerminateWithContext terminates the application with context.
	TerminateWithContext(ctx context.Context) error

	// GetConfig returns the configuration instance.
	GetConfig() Config

	// GetLogger returns the logger instance.
	GetLogger() Logger

	// BindValue binds a value to the container by name.
	BindValue(name string, value any) error
}
