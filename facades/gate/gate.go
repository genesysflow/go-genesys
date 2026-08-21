// Package gate provides a static facade for the authorization gate.
package gate

import (
	"sync"

	baseauth "github.com/genesysflow/go-genesys/auth"
)

var (
	instance *baseauth.Gate
	mu       sync.RWMutex
)

// SetInstance sets the gate instance.
// This is called during application bootstrap.
func SetInstance(gate *baseauth.Gate) {
	mu.Lock()
	defer mu.Unlock()
	instance = gate
}

// GetInstance returns the gate instance.
func GetInstance() *baseauth.Gate {
	mu.RLock()
	defer mu.RUnlock()
	return instance
}

func gate() *baseauth.Gate {
	mu.RLock()
	defer mu.RUnlock()
	if instance == nil {
		panic("gate: facade not initialised - register the AuthServiceProvider")
	}
	return instance
}

// Define registers an ability.
func Define(ability string, fn baseauth.AbilityFunc) {
	gate().Define(ability, fn)
}

// Before registers a hook that runs before every check.
func Before(fn func(user baseauth.Authenticatable, ability string, args ...any) *bool) {
	gate().Before(fn)
}

// Allows reports whether the user may perform the ability.
func Allows(user baseauth.Authenticatable, ability string, args ...any) bool {
	return gate().Allows(user, ability, args...)
}

// Denies reports whether the user may not perform the ability.
func Denies(user baseauth.Authenticatable, ability string, args ...any) bool {
	return gate().Denies(user, ability, args...)
}

// Authorize returns a 403 error unless the user may perform the ability.
func Authorize(user baseauth.Authenticatable, ability string, args ...any) error {
	return gate().Authorize(user, ability, args...)
}
