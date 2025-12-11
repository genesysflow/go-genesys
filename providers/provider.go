// Package providers implements the service provider pattern.
// Service providers are responsible for bootstrapping application services.
package providers

import (
	"github.com/genesysflow/go-genesys/contracts"
)

// ServiceProvider is the interface that all service providers must implement.
type ServiceProvider interface {
	contracts.ServiceProvider
}

// BaseProvider provides common functionality for service providers.
type BaseProvider struct {
	app contracts.Application
}

// SetApp sets the application instance.
func (p *BaseProvider) SetApp(app contracts.Application) {
	p.app = app
}

// App returns the application instance.
func (p *BaseProvider) App() contracts.Application {
	return p.app
}

// Register is called to bind services into the container.
// Override this method in your provider.
func (p *BaseProvider) Register(app contracts.Application) error {
	p.app = app
	return nil
}

// Boot is called after all providers have been registered.
// Override this method in your provider.
func (p *BaseProvider) Boot(app contracts.Application) error {
	return nil
}

// Provides returns a list of services this provider registers.
// Override this method in your provider.
func (p *BaseProvider) Provides() []string {
	return []string{}
}

// DeferrableProvider is a provider that can defer its registration.
type DeferrableProvider struct {
	BaseProvider
	deferred bool
}

// IsDeferred returns true if this provider should be deferred.
func (p *DeferrableProvider) IsDeferred() bool {
	return p.deferred
}

// SetDeferred sets whether this provider is deferred.
func (p *DeferrableProvider) SetDeferred(deferred bool) {
	p.deferred = deferred
}

// ProviderRegistry keeps track of registered providers.
type ProviderRegistry struct {
	providers       []contracts.ServiceProvider
	registered      map[string]bool
	booted          map[string]bool
	deferredLoading map[string]contracts.ServiceProvider
}

// NewRegistry creates a new provider registry.
func NewRegistry() *ProviderRegistry {
	return &ProviderRegistry{
		providers:       make([]contracts.ServiceProvider, 0),
		registered:      make(map[string]bool),
		booted:          make(map[string]bool),
		deferredLoading: make(map[string]contracts.ServiceProvider),
	}
}

// Register adds a provider to the registry.
func (r *ProviderRegistry) Register(provider contracts.ServiceProvider) {
	r.providers = append(r.providers, provider)
}

// All returns all registered providers.
func (r *ProviderRegistry) All() []contracts.ServiceProvider {
	return r.providers
}

// IsRegistered checks if a provider type is registered.
func (r *ProviderRegistry) IsRegistered(name string) bool {
	return r.registered[name]
}

// MarkRegistered marks a provider as registered.
func (r *ProviderRegistry) MarkRegistered(name string) {
	r.registered[name] = true
}

// IsBooted checks if a provider type is booted.
func (r *ProviderRegistry) IsBooted(name string) bool {
	return r.booted[name]
}

// MarkBooted marks a provider as booted.
func (r *ProviderRegistry) MarkBooted(name string) {
	r.booted[name] = true
}

// AddDeferred adds a deferred provider for lazy loading.
func (r *ProviderRegistry) AddDeferred(service string, provider contracts.ServiceProvider) {
	r.deferredLoading[service] = provider
}

// GetDeferred returns the deferred provider for a service.
func (r *ProviderRegistry) GetDeferred(service string) (contracts.ServiceProvider, bool) {
	p, ok := r.deferredLoading[service]
	return p, ok
}
