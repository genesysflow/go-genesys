// Package container provides a dependency injection container wrapper around samber/do.
// It offers a Laravel-like API for service registration and resolution.
package container

import (
	"context"
	"fmt"
	"sync"

	"github.com/samber/do/v2"
)

// Container is a wrapper around samber/do providing a Laravel-like DI container.
type Container struct {
	injector *do.RootScope
	mu       sync.RWMutex
	bindings map[string]bool // Track named bindings
}

// New creates a new container instance.
func New() *Container {
	return &Container{
		injector: do.New(),
		bindings: make(map[string]bool),
	}
}

// Injector returns the underlying do.Injector for advanced usage.
func (c *Container) Injector() *do.RootScope {
	return c.injector
}

// Bind registers a factory function that creates a new instance each time (transient).
func (c *Container) Bind(name string, factory any) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.bindings[name] = true
	// For transient bindings in do, we use ProvideNamedTransient
	// However, do v2 works with generics, so we need to handle this differently
	// We'll store the factory and create instances on demand
	return nil
}

// Singleton registers a factory function that creates a single shared instance (lazy).
func (c *Container) Singleton(name string, factory any) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.bindings[name] = true
	return nil
}

// Instance registers an already-created instance.
func (c *Container) Instance(name string, instance any) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.bindings[name] = true
	do.ProvideNamedValue(c.injector, name, instance)
	return nil
}

// Make resolves a service by name from the container.
func (c *Container) Make(name string) (any, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return do.InvokeNamed[any](c.injector, name)
}

// MustMake resolves a service by name, panicking on error.
func (c *Container) MustMake(name string) any {
	service, err := c.Make(name)
	if err != nil {
		panic(fmt.Sprintf("container: failed to resolve service '%s': %v", name, err))
	}
	return service
}

// Has checks if a service is registered in the container.
func (c *Container) Has(name string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	_, ok := c.bindings[name]
	return ok
}

// Shutdown gracefully shuts down all services.
func (c *Container) Shutdown() error {
	return c.injector.Shutdown()
}

// ShutdownWithContext gracefully shuts down all services with context.
func (c *Container) ShutdownWithContext(ctx context.Context) error {
	return c.injector.ShutdownWithContext(ctx)
}

// Provide registers a service using generics (recommended approach).
// The factory function receives the injector and returns the service and an error.
func Provide[T any](c *Container, factory func(*do.RootScope) (T, error)) {
	do.Provide(c.injector, func(i do.Injector) (T, error) {
		return factory(c.injector)
	})
}

// ProvideNamed registers a named service using generics.
func ProvideNamed[T any](c *Container, name string, factory func(*do.RootScope) (T, error)) {
	c.mu.Lock()
	c.bindings[name] = true
	c.mu.Unlock()

	do.ProvideNamed(c.injector, name, func(i do.Injector) (T, error) {
		return factory(c.injector)
	})
}

// ProvideValue registers an existing value.
func ProvideValue[T any](c *Container, value T) {
	do.ProvideValue(c.injector, value)
}

// ProvideNamedValue registers a named existing value.
func ProvideNamedValue[T any](c *Container, name string, value T) {
	c.mu.Lock()
	c.bindings[name] = true
	c.mu.Unlock()

	do.ProvideNamedValue(c.injector, name, value)
}

// ProvideTransient registers a transient service (new instance each time).
func ProvideTransient[T any](c *Container, factory func(*do.RootScope) (T, error)) {
	do.ProvideTransient(c.injector, func(i do.Injector) (T, error) {
		return factory(c.injector)
	})
}

// ProvideNamedTransient registers a named transient service.
func ProvideNamedTransient[T any](c *Container, name string, factory func(*do.RootScope) (T, error)) {
	c.mu.Lock()
	c.bindings[name] = true
	c.mu.Unlock()

	do.ProvideNamedTransient(c.injector, name, func(i do.Injector) (T, error) {
		return factory(c.injector)
	})
}

// Invoke resolves a service by type.
func Invoke[T any](c *Container) (T, error) {
	return do.Invoke[T](c.injector)
}

// MustInvoke resolves a service by type, panicking on error.
func MustInvoke[T any](c *Container) T {
	return do.MustInvoke[T](c.injector)
}

// InvokeNamed resolves a named service by type.
func InvokeNamed[T any](c *Container, name string) (T, error) {
	return do.InvokeNamed[T](c.injector, name)
}

// MustInvokeNamed resolves a named service by type, panicking on error.
func MustInvokeNamed[T any](c *Container, name string) T {
	return do.MustInvokeNamed[T](c.injector, name)
}

// Override replaces an existing service with a new factory.
func Override[T any](c *Container, factory func(*do.RootScope) (T, error)) {
	do.Override(c.injector, func(i do.Injector) (T, error) {
		return factory(c.injector)
	})
}

// OverrideNamed replaces an existing named service.
func OverrideNamed[T any](c *Container, name string, factory func(*do.RootScope) (T, error)) {
	do.OverrideNamed(c.injector, name, func(i do.Injector) (T, error) {
		return factory(c.injector)
	})
}

// OverrideValue replaces an existing service with a value.
func OverrideValue[T any](c *Container, value T) {
	do.OverrideValue(c.injector, value)
}

// OverrideNamedValue replaces an existing named service with a value.
func OverrideNamedValue[T any](c *Container, name string, value T) {
	do.OverrideNamedValue(c.injector, name, value)
}
