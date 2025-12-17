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

	// Register the factory to be invoked on demand
	if c.bindings[name] {
		do.OverrideNamedTransient(c.injector, name, func(i do.Injector) (any, error) {
			return c.invokeFactory(factory)
		})
	} else {
		c.bindings[name] = true
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
	if c.bindings[name] {
		do.OverrideNamed(c.injector, name, func(i do.Injector) (any, error) {
			return c.invokeFactory(factory)
		})
	} else {
		c.bindings[name] = true
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

	// Handle pointer types to get the full package path
	if out.Kind() == reflect.Ptr {
		elem := out.Elem()
		if elem.PkgPath() != "" {
			return "*" + elem.PkgPath() + "." + elem.Name(), nil
		}
	} else {
		if out.PkgPath() != "" {
			return out.PkgPath() + "." + out.Name(), nil
		}
	}

	return out.String(), nil
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
		var serviceName string
		if argType.Kind() == reflect.Ptr {
			elem := argType.Elem()
			if elem.PkgPath() != "" {
				serviceName = "*" + elem.PkgPath() + "." + elem.Name()
			} else {
				serviceName = argType.String()
			}
		} else {
			if argType.PkgPath() != "" {
				serviceName = argType.PkgPath() + "." + argType.Name()
			} else {
				serviceName = argType.String()
			}
		}

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

	if c.bindings[name] {
		do.OverrideNamedValue(c.injector, name, instance)
	} else {
		c.bindings[name] = true
		do.ProvideNamedValue(c.injector, name, instance)
	}
	return nil
}

// InstanceType registers an already-created instance, inferring the service name from its type.
func (c *Container) InstanceType(instance any) error {
	t := reflect.TypeOf(instance)
	name := t.String()
	return c.Instance(name, instance)
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

// Resolve resolves a service by name from the container and casts it to T.
// It accepts any contracts.Container, allowing usage with Application directly.
// If name is empty, it infers the service name from T.
func Resolve[T any](c contracts.Container, name ...string) (T, error) {
	if len(name) == 0 || name[0] == "" {
		// Try to use do.Invoke[T] directly if possible
		if containerImpl, ok := c.(*Container); ok {
			containerImpl.mu.RLock()
			defer containerImpl.mu.RUnlock()
			return do.Invoke[T](containerImpl.injector)
		}
	}

	var serviceName string
	if len(name) > 0 && name[0] != "" {
		serviceName = name[0]
	} else {
		// Infer name from T
		typeOfT := reflect.TypeOf((*T)(nil)).Elem()
		if typeOfT.Kind() == reflect.Ptr {
			elem := typeOfT.Elem()
			if elem.PkgPath() != "" {
				serviceName = "*" + elem.PkgPath() + "." + elem.Name()
			} else {
				serviceName = typeOfT.String()
			}
		} else {
			if typeOfT.PkgPath() != "" {
				serviceName = typeOfT.PkgPath() + "." + typeOfT.Name()
			} else {
				serviceName = typeOfT.String()
			}
		}
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
