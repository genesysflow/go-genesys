// Package foundation provides the core application bootstrapping.
// It orchestrates the entire framework lifecycle similar to Laravel.
package foundation

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"

	"github.com/genesysflow/go-genesys/config"
	"github.com/genesysflow/go-genesys/container"
	"github.com/genesysflow/go-genesys/contracts"
	"github.com/genesysflow/go-genesys/env"
	"github.com/genesysflow/go-genesys/log"
	"github.com/genesysflow/go-genesys/providers"
	"github.com/samber/do/v2"
)

const Version = "1.0.8"

// Application is the main application container.
// It orchestrates the entire framework lifecycle.
type Application struct {
	*container.Container

	basePath    string
	environment string
	debug       bool
	booted      bool

	providers *providers.ProviderRegistry
	config    *config.Config
	logger    contracts.Logger

	bootingCallbacks    []func(contracts.Application)
	bootedCallbacks     []func(contracts.Application)
	terminatingCallback []func(contracts.Application)

	mu sync.RWMutex
}

// New creates a new Application instance.
func New(basePath ...string) *Application {
	app := &Application{
		Container:           container.New(),
		providers:           providers.NewRegistry(),
		config:              config.New(),
		bootingCallbacks:    make([]func(contracts.Application), 0),
		bootedCallbacks:     make([]func(contracts.Application), 0),
		terminatingCallback: make([]func(contracts.Application), 0),
	}

	// Set base path
	if len(basePath) > 0 {
		app.basePath = basePath[0]
	} else {
		// Default to current working directory
		cwd, err := os.Getwd()
		if err == nil {
			app.basePath = cwd
		}
	}

	// Register the application itself
	app.registerBaseBindings()

	// Load environment
	app.loadEnvironment()

	// Set environment from ENV variable
	app.environment = env.Get("APP_ENV", "local")
	app.debug = env.GetBool("APP_DEBUG", true)

	return app
}

// registerBaseBindings registers the core bindings.
func (app *Application) registerBaseBindings() {
	// Register the application itself
	app.InstanceType(app)
	app.Instance("app", app)
	// Also register as the interface
	container.ProvideValue[contracts.Application](app.Container, app)

	// Register config
	app.InstanceType(app.config)
	app.Instance("config", app.config)
	container.ProvideValue[contracts.Config](app.Container, app.config)

	// Register default logger
	app.logger = log.New()
	app.InstanceType(app.logger) // This might register as *log.Logger
	app.Instance("logger", app.logger)
	container.ProvideValue[contracts.Logger](app.Container, app.logger)
}

// loadEnvironment loads environment variables from .env files.
func (app *Application) loadEnvironment() {
	// Load .env file if it exists
	envPath := filepath.Join(app.basePath, ".env")
	env.LoadIfExists(envPath)

	// Load environment-specific .env file
	appEnv := env.Get("APP_ENV", "local")
	envPath = filepath.Join(app.basePath, ".env."+appEnv)
	env.LoadIfExists(envPath)
}

// Version returns the framework version.
func (app *Application) Version() string {
	return Version
}

// BasePath returns the base path of the application.
func (app *Application) BasePath() string {
	return app.basePath
}

// SetBasePath sets the base path of the application.
func (app *Application) SetBasePath(path string) contracts.Application {
	app.basePath = path
	return app
}

// ConfigPath returns the path to the config directory.
func (app *Application) ConfigPath() string {
	return filepath.Join(app.basePath, "config")
}

// StoragePath returns the path to the storage directory.
func (app *Application) StoragePath() string {
	return filepath.Join(app.basePath, "storage")
}

// Environment returns the current environment.
func (app *Application) Environment() string {
	return app.environment
}

// IsEnvironment checks if the app is running in the given environment(s).
func (app *Application) IsEnvironment(envs ...string) bool {
	return slices.Contains(envs, app.environment)
}

// IsProduction checks if the app is running in production.
func (app *Application) IsProduction() bool {
	return app.environment == "production"
}

// IsLocal checks if the app is running locally.
func (app *Application) IsLocal() bool {
	return app.environment == "local"
}

// IsDebug checks if debug mode is enabled.
func (app *Application) IsDebug() bool {
	return app.debug
}

// Config returns the configuration instance.
func (app *Application) Config() *config.Config {
	return app.config
}

// Logger returns the logger instance.
func (app *Application) Logger() contracts.Logger {
	return app.logger
}

// GetConfig returns the configuration instance (implements contracts.Application).
func (app *Application) GetConfig() contracts.Config {
	return app.config
}

// GetLogger returns the logger instance (implements contracts.Application).
func (app *Application) GetLogger() contracts.Logger {
	return app.logger
}

// BindValue binds a value to the container by name.
func (app *Application) BindValue(name string, value any) error {
	return app.Instance(name, value)
}

// SetLogger sets the logger instance.
func (app *Application) SetLogger(logger contracts.Logger) {
	app.logger = logger
	container.OverrideValue[contracts.Logger](app.Container, logger)
}

// Register registers a service provider with the application.
func (app *Application) Register(provider contracts.ServiceProvider) error {
	app.mu.Lock()
	defer app.mu.Unlock()

	// Get provider name for tracking
	t := reflect.TypeOf(provider)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	providerName := t.PkgPath() + "." + t.Name()

	// Check if already registered
	if app.providers.IsRegistered(providerName) {
		return nil
	}

	// Add to registry
	app.providers.Register(provider)

	// Check if this is a deferrable provider
	if deferrable, ok := provider.(contracts.DeferrableProvider); ok && deferrable.IsDeferred() {
		// Register for deferred loading
		for _, service := range provider.Provides() {
			app.providers.AddDeferred(service, provider)
		}
		app.providers.MarkRegistered(providerName)
		return nil
	}

	// Call the provider's Register method
	if err := provider.Register(app); err != nil {
		return fmt.Errorf("failed to register provider %s: %w", providerName, err)
	}

	app.providers.MarkRegistered(providerName)

	// If the application is already booted, boot this provider immediately
	if app.booted {
		if err := app.bootProvider(provider); err != nil {
			return err
		}
	}

	return nil
}

// bootProvider boots a single provider.
func (app *Application) bootProvider(provider contracts.ServiceProvider) error {
	providerName := reflect.TypeOf(provider).String()

	if app.providers.IsBooted(providerName) {
		return nil
	}

	// Check if provider should be booted
	if bootable, ok := provider.(contracts.BootableProvider); ok {
		if !bootable.ShouldBoot() {
			return nil
		}
	}

	if err := provider.Boot(app); err != nil {
		return fmt.Errorf("failed to boot provider %s: %w", providerName, err)
	}

	app.providers.MarkBooted(providerName)
	return nil
}

// Boot boots the application and all registered providers.
func (app *Application) Boot() error {
	app.mu.Lock()

	if app.booted {
		app.mu.Unlock()
		return nil
	}

	// Load configuration
	configPath := app.ConfigPath()
	if _, err := os.Stat(configPath); err == nil {
		if err := app.config.Load(configPath); err != nil {
			app.mu.Unlock()
			return fmt.Errorf("failed to load config: %w", err)
		}
	}

	// Run booting callbacks
	for _, callback := range app.bootingCallbacks {
		callback(app)
	}

	app.mu.Unlock()

	// Boot all providers
	for _, provider := range app.providers.All() {
		if err := app.bootProvider(provider); err != nil {
			return err
		}
	}

	app.mu.Lock()
	app.booted = true

	// Run booted callbacks
	for _, callback := range app.bootedCallbacks {
		callback(app)
	}
	app.mu.Unlock()

	return nil
}

// IsBooted returns true if the application has been booted.
func (app *Application) IsBooted() bool {
	app.mu.RLock()
	defer app.mu.RUnlock()
	return app.booted
}

// Booting registers a callback to be run before booting.
func (app *Application) Booting(callback func(contracts.Application)) {
	app.mu.Lock()
	defer app.mu.Unlock()
	app.bootingCallbacks = append(app.bootingCallbacks, callback)
}

// Booted registers a callback to be run after booting.
func (app *Application) Booted(callback func(contracts.Application)) {
	app.mu.Lock()
	defer app.mu.Unlock()

	if app.booted {
		callback(app)
		return
	}

	app.bootedCallbacks = append(app.bootedCallbacks, callback)
}

// Terminating registers a callback to be run during termination.
func (app *Application) Terminating(callback func(contracts.Application)) {
	app.mu.Lock()
	defer app.mu.Unlock()
	app.terminatingCallback = append(app.terminatingCallback, callback)
}

// Terminate terminates the application.
func (app *Application) Terminate() error {
	return app.TerminateWithContext(context.Background())
}

// TerminateWithContext terminates the application with context.
func (app *Application) TerminateWithContext(ctx context.Context) error {
	app.mu.Lock()
	callbacks := app.terminatingCallback
	app.mu.Unlock()

	// Run terminating callbacks
	for _, callback := range callbacks {
		callback(app)
	}

	// Shutdown the container
	// Note: samber/do may return marshaling errors during shutdown which are harmless
	err := app.ShutdownWithContext(ctx)
	if err != nil {
		// Ignore JSON marshaling errors from samber/do - they don't affect shutdown
		if strings.Contains(err.Error(), "marshaling error") {
			return nil
		}
		return err
	}
	return nil
}

// Make resolves a service by name from the container.
func (app *Application) Make(name string) (any, error) {
	// Check for deferred provider
	if provider, ok := app.providers.GetDeferred(name); ok {
		providerName := reflect.TypeOf(provider).String()
		if !app.providers.IsRegistered(providerName) {
			if err := provider.Register(app); err != nil {
				return nil, err
			}
			app.providers.MarkRegistered(providerName)
			if err := app.bootProvider(provider); err != nil {
				return nil, err
			}
		}
	}

	return app.Container.Make(name)
}

// MustMake resolves a service by name, panicking on error.
func (app *Application) MustMake(name string) any {
	service, err := app.Make(name)
	if err != nil {
		panic(fmt.Sprintf("application: failed to resolve service '%s': %v", name, err))
	}
	return service
}

// Invoke resolves a service by type from the application container.
func Invoke[T any](app *Application) (T, error) {
	return container.Invoke[T](app.Container)
}

// MustInvoke resolves a service by type, panicking on error.
func MustInvoke[T any](app *Application) T {
	return container.MustInvoke[T](app.Container)
}

// Provide registers a service with the application container.
func Provide[T any](app *Application, factory func(*do.RootScope) (T, error)) {
	container.Provide[T](app.Container, factory)
}

// ProvideValue registers an existing value with the application container.
func ProvideValue[T any](app *Application, value T) {
	container.ProvideValue[T](app.Container, value)
}

// ProvideNamedValue registers a named existing value with the application container.
func ProvideNamedValue[T any](app *Application, name string, value T) {
	container.ProvideNamedValue[T](app.Container, name, value)
}

// ProvideTransient registers a transient service with the application container.
func ProvideTransient[T any](app *Application, factory func(*do.RootScope) (T, error)) {
	container.ProvideTransient[T](app.Container, factory)
}
