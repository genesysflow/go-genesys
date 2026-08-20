package auth

import (
	"errors"
	"fmt"

	"github.com/genesysflow/go-genesys/hash"
	"github.com/genesysflow/go-genesys/http"
	"github.com/genesysflow/go-genesys/session"
)

const sessionKey = "_auth_id"

// SessionGuard authenticates users via the session. It requires the session
// middleware to be registered on the kernel.
type SessionGuard struct {
	name     string
	provider UserProvider
}

// NewSessionGuard creates a session guard backed by the given user provider.
func NewSessionGuard(name string, provider UserProvider) *SessionGuard {
	return &SessionGuard{name: name, provider: provider}
}

func (g *SessionGuard) session(ctx *http.Context) *session.Session {
	return session.GetFromContext(ctx.FiberCtx())
}

func (g *SessionGuard) cacheKey() string {
	return "_auth_user_" + g.name
}

// Attempt verifies the credentials and logs the user in on success. The
// "password" key is checked against the user's hashed password; all other
// keys locate the user.
func (g *SessionGuard) Attempt(ctx *http.Context, credentials map[string]any) (Authenticatable, error) {
	password, _ := credentials["password"].(string)
	lookup := make(map[string]any, len(credentials))
	for k, v := range credentials {
		if k != "password" {
			lookup[k] = v
		}
	}

	user, err := g.provider.RetrieveByCredentials(lookup)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			// Burn a hash comparison anyway so response timing does not
			// reveal whether the account exists.
			hash.Check(password, "$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy")
			return nil, ErrInvalidCredentials
		}
		return nil, err
	}

	if err := hash.Check(password, user.GetAuthPassword()); err != nil {
		if errors.Is(err, hash.ErrMismatch) {
			return nil, ErrInvalidCredentials
		}
		return nil, err
	}

	if err := g.Login(ctx, user); err != nil {
		return nil, err
	}
	return user, nil
}

// Login authenticates the given user for the session. The session ID is
// regenerated to prevent session fixation.
func (g *SessionGuard) Login(ctx *http.Context, user Authenticatable) error {
	sess := g.session(ctx)
	if sess == nil {
		return fmt.Errorf("auth: no session on request - register the session middleware before using the session guard")
	}
	if err := sess.Regenerate(); err != nil {
		return err
	}
	if err := sess.Set(sessionKey, fmt.Sprint(user.GetAuthIdentifier())); err != nil {
		return err
	}
	ctx.Set(g.cacheKey(), user)
	return nil
}

// Logout ends the authenticated session.
func (g *SessionGuard) Logout(ctx *http.Context) error {
	sess := g.session(ctx)
	if sess == nil {
		return nil
	}
	if err := sess.Forget(sessionKey); err != nil {
		return err
	}
	ctx.Set(g.cacheKey(), nil)
	return sess.Regenerate()
}

// User returns the authenticated user, or nil. The lookup is cached for
// the rest of the request.
func (g *SessionGuard) User(ctx *http.Context) Authenticatable {
	if cached := ctx.Get(g.cacheKey()); cached != nil {
		if user, ok := cached.(Authenticatable); ok {
			return user
		}
		return nil
	}

	sess := g.session(ctx)
	if sess == nil {
		return nil
	}
	id := sess.Get(sessionKey)
	if id == nil {
		return nil
	}

	user, err := g.provider.RetrieveByID(id)
	if err != nil {
		return nil
	}
	ctx.Set(g.cacheKey(), user)
	return user
}

// Check reports whether the request is authenticated.
func (g *SessionGuard) Check(ctx *http.Context) bool {
	return g.User(ctx) != nil
}

// ID returns the authenticated user's identifier, or nil.
func (g *SessionGuard) ID(ctx *http.Context) any {
	if user := g.User(ctx); user != nil {
		return user.GetAuthIdentifier()
	}
	return nil
}
