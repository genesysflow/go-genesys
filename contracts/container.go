// Package contracts defines interfaces for all major framework components.
// These contracts enable dependency injection and make the framework testable.
package contracts

import "context"

// Container defines the interface for the dependency injection container.
type Container interface {
	// Bind registers a factory function that creates a new instance each time.
	Bind(name string, factory any) error

	// Singleton registers a factory function that creates a single shared instance.
	Singleton(name string, factory any) error

	// BindType registers a factory function, inferring the service name from the return type.
	BindType(factory any) error

	// SingletonType registers a singleton factory, inferring the service name from the return type.
	SingletonType(factory any) error

	// Instance registers an already-created instance.
	Instance(name string, instance any) error

	// InstanceType registers an already-created instance, inferring the service name from its type.
	InstanceType(instance any) error

	// Make resolves a service by name from the container.
	Make(name string) (any, error)

	// MustMake resolves a service by name, panicking on error.
	MustMake(name string) any

	// Has checks if a service is registered in the container.
	Has(name string) bool

	// Shutdown gracefully shuts down all services.
	Shutdown() error

	// ShutdownWithContext gracefully shuts down all services with context.
	ShutdownWithContext(ctx context.Context) error
}

// ContainerAware is implemented by types that need access to the container.
type ContainerAware interface {
	SetContainer(container Container)
	Container() Container
}
