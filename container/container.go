// Package container provides a dependency injection container wrapper around samber/do.
// It offers a Laravel-like API for service registration and resolution.
package container

import (
	"context"
	"fmt"
	"reflect"
	"sync"

	"github.com/genesysflow/go-genesys/contracts"
	"github.com/samber/do/v2"
)

// Container is a wrapper around samber/do providing a Laravel-like DI container.
type Container struct {
	injector *do.RootScope
	// mu serialises the check-then-register sequence of Bind, Singleton
	// and Instance so two goroutines cannot both decide a name is free
	// and race into do.ProvideNamed*, which panics on a duplicate. The
	// injector itself is internally synchronised and is the single
	// source of truth for which services exist.
	mu sync.Mutex
}

// New creates a new container instance.
func New() *Container {
	return &Container{
		injector: do.New(),
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

	// Register the factory to be invoked on demand
	if c.Has(name) {
		do.OverrideNamedTransient(c.injector, name, func(i do.Injector) (any, error) {
			return c.invokeFactory(factory)
		})
	} else {
		do.ProvideNamedTransient(c.injector, name, func(i do.Injector) (any, error) {
			return c.invokeFactory(factory)
		})
	}
	return nil
}

// Singleton registers a factory function that creates a single shared instance (lazy).
func (c *Container) Singleton(name string, factory any) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Register the factory as a singleton
	if c.Has(name) {
		do.OverrideNamed(c.injector, name, func(i do.Injector) (any, error) {
			return c.invokeFactory(factory)
		})
	} else {
		do.ProvideNamed(c.injector, name, func(i do.Injector) (any, error) {
			return c.invokeFactory(factory)
		})
	}
	return nil
}

// BindType registers a factory function, inferring the service name from the return type.
func (c *Container) BindType(factory any) error {
	name, err := inferServiceName(factory)
	if err != nil {
		return err
	}
	return c.Bind(name, factory)
}

// SingletonType registers a singleton factory, inferring the service name from the return type.
func (c *Container) SingletonType(factory any) error {
	name, err := inferServiceName(factory)
	if err != nil {
		return err
	}
	return c.Singleton(name, factory)
}

// inferServiceName infers the service name from the factory's return type.
func inferServiceName(factory any) (string, error) {
	val := reflect.ValueOf(factory)
	if val.Kind() != reflect.Func {
		return "", fmt.Errorf("container: factory must be a function")
	}
	t := val.Type()
	if t.NumOut() == 0 {
		return "", fmt.Errorf("container: factory must return at least one value")
	}
	// Use the first return value's type as the name
	out := t.Out(0)
	return GetTypeName(out), nil
}

// invokeFactory executes the given factory function, injecting the container if needed.
func (c *Container) invokeFactory(factory any) (any, error) {
	val := reflect.ValueOf(factory)

	// If it's not a function, return the value as is
	if val.Kind() != reflect.Func {
		return factory, nil
	}

	t := val.Type()
	args := make([]reflect.Value, t.NumIn())

	for i := 0; i < t.NumIn(); i++ {
		argType := t.In(i)

		// 1. Check for Container injection
		// Only inject if the container instance itself is assignable to the argument type
		// This prevents injecting *Container when *Application is requested
		if reflect.TypeOf(c).AssignableTo(argType) {
			args[i] = reflect.ValueOf(c)
			continue
		}

		// 2. Resolve by Type
		// samber/do uses the type string as the service name when using Provide[T].
		serviceName := GetTypeName(argType)

		// Check if we have it
		instance, err := c.Make(serviceName)
		if err != nil {
			return nil, fmt.Errorf("container: failed to resolve dependency '%s' (type %s): %w", serviceName, argType, err)
		}
		args[i] = reflect.ValueOf(instance)
	}

	results := val.Call(args)

	if len(results) == 0 {
		return nil, nil
	}

	// Check if the last return value is an error
	last := results[len(results)-1]
	if last.Type().Implements(reflect.TypeOf((*error)(nil)).Elem()) {
		if !last.IsNil() {
			return nil, last.Interface().(error)
		}
		if len(results) > 1 {
			return results[0].Interface(), nil
		}
		return nil, nil
	}

	return results[0].Interface(), nil
}

// Call invokes a function, injecting its dependencies.
// To support full auto-wiring, you would need to map types to do.Invoke calls.
func (c *Container) Call(function any) ([]any, error) {
	val := reflect.ValueOf(function)
	if val.Kind() != reflect.Func {
		return nil, fmt.Errorf("container: Call expected a function, got %T", function)
	}

	// Reuse invokeFactory logic or expand it to handle more arguments
	res, err := c.invokeFactory(function)
	if err != nil {
		return nil, err
	}
	return []any{res}, nil
}

// Instance registers an already-created instance.
func (c *Container) Instance(name string, instance any) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.Has(name) {
		do.OverrideNamedValue(c.injector, name, instance)
	} else {
		do.ProvideNamedValue(c.injector, name, instance)
	}
	return nil
}

// InstanceType registers an already-created instance, inferring the service name from its type.
func (c *Container) InstanceType(instance any) error {
	t := reflect.TypeOf(instance)
	name := GetTypeName(t)
	return c.Instance(name, instance)
}

// GetTypeName returns the fully qualified type name for a type.
func GetTypeName(t reflect.Type) string {
	if t.Kind() == reflect.Ptr {
		elem := t.Elem()
		if elem.PkgPath() != "" {
			return "*" + elem.PkgPath() + "." + elem.Name()
		}
		return t.String()
	}
	if t.PkgPath() != "" {
		return t.PkgPath() + "." + t.Name()
	}
	return t.String()
}

// Make resolves a service by name from the container. It deliberately
// holds no container lock: samber/do is internally synchronized, and a
// read lock here would deadlock lazy factories that resolve their own
// dependencies through a nested Make once a writer (Bind/Instance) is
// queued - Go's RWMutex blocks re-entrant RLock in that case.
func (c *Container) Make(name string) (any, error) {
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

// Has checks if a service is registered in the container. It asks the
// injector rather than keeping a parallel ledger, so it agrees with Make
// for every registration path - including the generic Provide*/Override*
// helpers, which register under the inferred type name - and reports
// false again once Shutdown has torn the services down. It takes no
// container lock for the same reason Make does not.
func (c *Container) Has(name string) bool {
	for _, service := range c.injector.ListProvidedServices() {
		if service.Service == name {
			return true
		}
	}
	return false
}

// Shutdown gracefully shuts down all services. It returns nil when every
// shutdown hook succeeded; otherwise it returns do's *ShutdownReport,
// which implements error and describes the failing services.
func (c *Container) Shutdown() error {
	return shutdownError(c.injector.Shutdown())
}

// ShutdownWithContext gracefully shuts down all services with context.
// It follows the same nil-on-success contract as Shutdown.
func (c *Container) ShutdownWithContext(ctx context.Context) error {
	return shutdownError(c.injector.ShutdownWithContext(ctx))
}

// shutdownError converts do's shutdown report into an idiomatic error.
// do hands back a non-nil *ShutdownReport even when nothing failed, so
// returning it verbatim would make `if err := c.Shutdown(); err != nil`
// fire on every successful shutdown.
func shutdownError(report *do.ShutdownReport) error {
	if report == nil || (report.Succeed && len(report.Errors) == 0) {
		return nil
	}
	return report
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

// Resolve resolves a service by name from the container and casts it to T.
// It accepts any contracts.Container, allowing usage with Application directly.
// If name is empty, it infers the service name from T.
func Resolve[T any](c contracts.Container, name ...string) (T, error) {
	var serviceName string
	if len(name) > 0 && name[0] != "" {
		serviceName = name[0]
	} else {
		// Infer name from T
		typeOfT := reflect.TypeOf((*T)(nil)).Elem()
		serviceName = GetTypeName(typeOfT)
	}

	instance, err := c.Make(serviceName)
	if err != nil {
		var zero T
		return zero, err
	}

	typed, ok := instance.(T)
	if !ok {
		var zero T
		return zero, fmt.Errorf("container: service '%s' is not of type %T", serviceName, zero)
	}

	return typed, nil
}

// MustResolve resolves a service by name, panicking on error.
// If name is empty, it infers the service name from T.
func MustResolve[T any](c contracts.Container, name ...string) T {
	instance, err := Resolve[T](c, name...)
	if err != nil {
		panic(err)
	}
	return instance
}
