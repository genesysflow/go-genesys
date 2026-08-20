package auth

import (
	"sync"

	"github.com/genesysflow/go-genesys/errors"
)

// AbilityFunc decides whether a user may perform an ability. args carry
// the subject (e.g. the model instance) when relevant.
type AbilityFunc func(user Authenticatable, args ...any) bool

// Gate authorizes user abilities, mirroring Laravel's Gate.
type Gate struct {
	abilities map[string]AbilityFunc
	before    []func(user Authenticatable, ability string, args ...any) *bool
	mu        sync.RWMutex
}

// NewGate creates an empty gate.
func NewGate() *Gate {
	return &Gate{abilities: make(map[string]AbilityFunc)}
}

// Define registers an ability:
//
//	gate.Define("update-post", func(user auth.Authenticatable, args ...any) bool {
//	    post := args[0].(*Post)
//	    return post.AuthorID == user.GetAuthIdentifier()
//	})
func (g *Gate) Define(ability string, fn AbilityFunc) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.abilities[ability] = fn
}

// Before registers a hook that runs before every check. Returning a
// non-nil result short-circuits the decision (e.g. admins may do anything).
func (g *Gate) Before(fn func(user Authenticatable, ability string, args ...any) *bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.before = append(g.before, fn)
}

// Allows reports whether the user may perform the ability.
func (g *Gate) Allows(user Authenticatable, ability string, args ...any) bool {
	g.mu.RLock()
	fn, defined := g.abilities[ability]
	before := append([]func(Authenticatable, string, ...any) *bool(nil), g.before...)
	g.mu.RUnlock()

	for _, hook := range before {
		if result := hook(user, ability, args...); result != nil {
			return *result
		}
	}
	if !defined || user == nil {
		return false
	}
	return fn(user, args...)
}

// Denies reports whether the user may not perform the ability.
func (g *Gate) Denies(user Authenticatable, ability string, args ...any) bool {
	return !g.Allows(user, ability, args...)
}

// Authorize returns a 403 error unless the user may perform the ability:
//
//	if err := gate.Authorize(user, "update-post", post); err != nil {
//	    return err
//	}
func (g *Gate) Authorize(user Authenticatable, ability string, args ...any) error {
	if g.Allows(user, ability, args...) {
		return nil
	}
	return errors.Forbidden("This action is unauthorized.")
}

// Any reports whether the user may perform any of the abilities.
func (g *Gate) Any(user Authenticatable, abilities []string, args ...any) bool {
	for _, ability := range abilities {
		if g.Allows(user, ability, args...) {
			return true
		}
	}
	return false
}
