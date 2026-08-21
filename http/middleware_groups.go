package http

import (
	"fmt"
	"sync"
)

// middlewareRegistry holds named middleware and middleware groups so routes
// can reference stacks by name, Laravel style.
type middlewareRegistry struct {
	aliases map[string]MiddlewareFunc
	groups  map[string][]MiddlewareFunc
	mu      sync.RWMutex
}

func (k *Kernel) registry() *middlewareRegistry {
	if k.mwRegistry == nil {
		k.mwRegistry = &middlewareRegistry{
			aliases: make(map[string]MiddlewareFunc),
			groups:  make(map[string][]MiddlewareFunc),
		}
	}
	return k.mwRegistry
}

// AliasMiddleware registers a named middleware:
//
//	kernel.AliasMiddleware("auth", auth.Middleware(guard))
//	kernel.GET("/profile", handler, kernel.Named("auth")...)
func (k *Kernel) AliasMiddleware(name string, middleware MiddlewareFunc) *Kernel {
	reg := k.registry()
	reg.mu.Lock()
	defer reg.mu.Unlock()
	reg.aliases[name] = middleware
	return k
}

// DefineMiddlewareGroup registers a named middleware stack:
//
//	kernel.DefineMiddlewareGroup("web", sessionMw, csrfMw)
//	kernel.Group("/admin", routes, kernel.MiddlewareGroup("web")...)
func (k *Kernel) DefineMiddlewareGroup(name string, middleware ...MiddlewareFunc) *Kernel {
	reg := k.registry()
	reg.mu.Lock()
	defer reg.mu.Unlock()
	reg.groups[name] = middleware
	return k
}

// MiddlewareGroup returns a named middleware stack; unknown names panic to
// fail fast during route registration.
func (k *Kernel) MiddlewareGroup(name string) []MiddlewareFunc {
	reg := k.registry()
	reg.mu.RLock()
	defer reg.mu.RUnlock()
	group, ok := reg.groups[name]
	if !ok {
		panic(fmt.Sprintf("http: middleware group [%s] is not defined", name))
	}
	return append([]MiddlewareFunc(nil), group...)
}

// Named resolves middleware aliases into a stack; unknown names panic to
// fail fast during route registration.
func (k *Kernel) Named(names ...string) []MiddlewareFunc {
	reg := k.registry()
	reg.mu.RLock()
	defer reg.mu.RUnlock()
	stack := make([]MiddlewareFunc, 0, len(names))
	for _, name := range names {
		mw, ok := reg.aliases[name]
		if !ok {
			panic(fmt.Sprintf("http: middleware alias [%s] is not defined", name))
		}
		stack = append(stack, mw)
	}
	return stack
}
