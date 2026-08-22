// Package auth provides Laravel-style authentication: guards (session,
// token), user providers, and authorization gates.
package auth

import (
	"errors"

	"github.com/genesysflow/go-genesys/http"
)

// ErrInvalidCredentials is returned by Attempt when the credentials do not
// match a user.
var ErrInvalidCredentials = errors.New("auth: invalid credentials")

// ErrUserNotFound is returned by user providers when no user matches.
var ErrUserNotFound = errors.New("auth: user not found")

// Authenticatable is implemented by user models that can be authenticated.
type Authenticatable interface {
	// GetAuthIdentifier returns the user's unique identifier (usually the
	// primary key).
	GetAuthIdentifier() any

	// GetAuthPassword returns the user's hashed password.
	GetAuthPassword() string
}

// UserProvider retrieves users for guards.
type UserProvider interface {
	// RetrieveByID returns the user with the given identifier, or
	// ErrUserNotFound.
	RetrieveByID(id any) (Authenticatable, error)

	// RetrieveByCredentials returns the user matching the non-password
	// credentials (e.g. {"email": ...}), or ErrUserNotFound. It must NOT
	// check the password; the guard does that.
	RetrieveByCredentials(credentials map[string]any) (Authenticatable, error)
}

// TokenUserProvider retrieves users by API token for the token guard.
type TokenUserProvider interface {
	// RetrieveByToken returns the user owning the given token, or
	// ErrUserNotFound.
	RetrieveByToken(token string) (Authenticatable, error)
}

// Guard authenticates requests.
type Guard interface {
	// Check reports whether the request is authenticated.
	Check(ctx *http.Context) bool

	// User returns the authenticated user, or nil.
	User(ctx *http.Context) Authenticatable

	// ID returns the authenticated user's identifier, or nil.
	ID(ctx *http.Context) any
}

// StatefulGuard is a guard that can log users in and out (sessions).
type StatefulGuard interface {
	Guard

	// Attempt logs a user in when the credentials (including "password")
	// match; returns ErrInvalidCredentials otherwise.
	Attempt(ctx *http.Context, credentials map[string]any) (Authenticatable, error)

	// Login authenticates the given user for the session.
	Login(ctx *http.Context, user Authenticatable) error

	// Logout ends the authenticated session.
	Logout(ctx *http.Context) error
}

// UserFrom returns the authenticated user the auth middleware (or a
// guard) stashed on the request, or nil when the request is
// unauthenticated. It is Laravel's `$request->user()` for guard-resolved
// users:
//
//	user := auth.UserFrom(ctx)
//	if user == nil { ... }
func UserFrom(ctx *http.Context) Authenticatable {
	user, ok := ctx.User().(Authenticatable)
	if !ok {
		return nil
	}
	return user
}
