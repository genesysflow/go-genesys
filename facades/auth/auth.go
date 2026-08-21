// Package auth provides a static facade for authentication guards.
package auth

import (
	"sync"

	baseauth "github.com/genesysflow/go-genesys/auth"
	"github.com/genesysflow/go-genesys/http"
)

var (
	instance *baseauth.Manager
	mu       sync.RWMutex
)

// SetInstance sets the auth manager instance.
// This is called during application bootstrap.
func SetInstance(manager *baseauth.Manager) {
	mu.Lock()
	defer mu.Unlock()
	instance = manager
}

// GetInstance returns the auth manager instance.
func GetInstance() *baseauth.Manager {
	mu.RLock()
	defer mu.RUnlock()
	return instance
}

func manager() *baseauth.Manager {
	mu.RLock()
	defer mu.RUnlock()
	if instance == nil {
		panic("auth: facade not initialised - register the AuthServiceProvider")
	}
	return instance
}

// Guard returns a guard by name (default guard when omitted).
func Guard(name ...string) baseauth.Guard {
	return manager().MustGuard(name...)
}

// User returns the authenticated user on the default guard, or nil.
func User(ctx *http.Context) baseauth.Authenticatable {
	return Guard().User(ctx)
}

// Check reports whether the request is authenticated on the default guard.
func Check(ctx *http.Context) bool {
	return Guard().Check(ctx)
}

// ID returns the authenticated user's identifier, or nil.
func ID(ctx *http.Context) any {
	return Guard().ID(ctx)
}

// Attempt logs a user in with credentials on the default guard, which must
// be stateful (session).
func Attempt(ctx *http.Context, credentials map[string]any) (baseauth.Authenticatable, error) {
	guard, ok := Guard().(baseauth.StatefulGuard)
	if !ok {
		panic("auth: default guard is not stateful - Attempt requires the session guard")
	}
	return guard.Attempt(ctx, credentials)
}

// Login authenticates the given user on the default guard.
func Login(ctx *http.Context, user baseauth.Authenticatable) error {
	guard, ok := Guard().(baseauth.StatefulGuard)
	if !ok {
		panic("auth: default guard is not stateful - Login requires the session guard")
	}
	return guard.Login(ctx, user)
}

// Logout ends the authenticated session on the default guard.
func Logout(ctx *http.Context) error {
	guard, ok := Guard().(baseauth.StatefulGuard)
	if !ok {
		panic("auth: default guard is not stateful - Logout requires the session guard")
	}
	return guard.Logout(ctx)
}
