package auth

import (
	"strings"

	"github.com/genesysflow/go-genesys/http"
)

// TokenGuard authenticates requests by bearer token (Authorization header)
// with an optional query-parameter fallback.
type TokenGuard struct {
	name     string
	provider TokenUserProvider

	// QueryParam, when set, allows the token to arrive as a query
	// parameter (e.g. "api_token") in addition to the bearer header.
	QueryParam string
}

// NewTokenGuard creates a token guard backed by the given provider.
func NewTokenGuard(name string, provider TokenUserProvider) *TokenGuard {
	return &TokenGuard{name: name, provider: provider}
}

func (g *TokenGuard) cacheKey() string {
	return "_auth_user_" + g.name
}

// TokenFromRequest extracts the API token from the request.
func (g *TokenGuard) TokenFromRequest(ctx *http.Context) string {
	header := ctx.FiberCtx().Get("Authorization")
	if strings.HasPrefix(header, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
	}
	if g.QueryParam != "" {
		return ctx.Query(g.QueryParam)
	}
	return ""
}

// User returns the authenticated user, or nil. The lookup is cached for
// the rest of the request.
func (g *TokenGuard) User(ctx *http.Context) Authenticatable {
	if cached := ctx.Get(g.cacheKey()); cached != nil {
		if user, ok := cached.(Authenticatable); ok {
			return user
		}
		return nil
	}

	token := g.TokenFromRequest(ctx)
	if token == "" {
		return nil
	}
	user, err := g.provider.RetrieveByToken(token)
	if err != nil {
		return nil
	}
	ctx.Set(g.cacheKey(), user)
	return user
}

// Check reports whether the request is authenticated.
func (g *TokenGuard) Check(ctx *http.Context) bool {
	return g.User(ctx) != nil
}

// ID returns the authenticated user's identifier, or nil.
func (g *TokenGuard) ID(ctx *http.Context) any {
	if user := g.User(ctx); user != nil {
		return user.GetAuthIdentifier()
	}
	return nil
}
