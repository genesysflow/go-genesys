package contracts

// ServiceProvider defines the interface for service providers.
// Service providers are the central place to configure and bootstrap application services.
type ServiceProvider interface {
	// Register is called to bind services into the container.
	// This method should only bind things into the container.
	// It should not attempt to use any other service.
	Register(app Application) error

	// Boot is called after all providers have been registered.
	// This method may use any service that has been registered.
	Boot(app Application) error

	// Providers returns a list of services this provider registers.
	// Used for debugging and introspection.
	Provides() []string
}

// DeferrableProvider is a service provider that can defer registration.
// Deferred providers are only loaded when one of their services is requested.
type DeferrableProvider interface {
	ServiceProvider

	// IsDeferred returns true if this provider should be deferred.
	IsDeferred() bool
}

// BootableProvider is a service provider with additional boot logic.
type BootableProvider interface {
	ServiceProvider

	// ShouldBoot returns true if this provider should run Boot().
	ShouldBoot() bool
}
