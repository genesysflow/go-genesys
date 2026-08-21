package auth_test

import (
	"io"
	"net/http/httptest"
	"testing"

	"github.com/genesysflow/go-genesys/auth"
	genhttp "github.com/genesysflow/go-genesys/http"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The auth middleware resolves the user once and stashes it on the
// context, so handlers reach it with ctx.User() instead of hitting the
// session or token store again.
func TestMiddlewareStashesUserOnContext(t *testing.T) {
	kernel, _, tokenGuard := setupAuthApp(t)

	kernel.GET("/stashed-user", func(ctx *genhttp.Context) error {
		require.True(t, ctx.HasUser())

		user, ok := genhttp.UserAs[User](ctx)
		require.True(t, ok, "stashed user should be the *User the provider returned")
		return ctx.String(user.Email)
	}, auth.Middleware(tokenGuard))

	req := httptest.NewRequest("GET", "/stashed-user", nil)
	req.Header.Set("Authorization", "Bearer token-123")
	resp, err := kernel.Fiber().Test(req, -1)
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	assert.Equal(t, "alice@example.com", string(body))
}

// auth.UserFrom is the typed accessor for guard-resolved users.
func TestUserFromReturnsAuthenticatable(t *testing.T) {
	kernel, _, tokenGuard := setupAuthApp(t)

	kernel.GET("/identifier", func(ctx *genhttp.Context) error {
		user := auth.UserFrom(ctx)
		require.NotNil(t, user)
		return ctx.JSONResponse(map[string]any{"id": user.GetAuthIdentifier()})
	}, auth.Middleware(tokenGuard))

	req := httptest.NewRequest("GET", "/identifier", nil)
	req.Header.Set("Authorization", "Bearer token-123")
	resp, err := kernel.Fiber().Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestUserFromReturnsNilWhenUnauthenticated(t *testing.T) {
	kernel, _, _ := setupAuthApp(t)

	kernel.GET("/anon", func(ctx *genhttp.Context) error {
		assert.Nil(t, auth.UserFrom(ctx))
		assert.False(t, ctx.HasUser())
		return ctx.String("ok")
	})

	resp, err := kernel.Fiber().Test(httptest.NewRequest("GET", "/anon", nil), -1)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

// A guard that reports Check() == true but resolves no user must not
// stash a typed-nil that would make HasUser() lie.
func TestMiddlewareDoesNotStashNilUser(t *testing.T) {
	kernel, _, _ := setupAuthApp(t)

	kernel.GET("/checked", func(ctx *genhttp.Context) error {
		assert.False(t, ctx.HasUser())
		assert.Nil(t, auth.UserFrom(ctx))
		return ctx.String("ok")
	}, auth.Middleware(&userlessGuard{}))

	resp, err := kernel.Fiber().Test(httptest.NewRequest("GET", "/checked", nil), -1)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

// userlessGuard passes the check but has no user to hand back.
type userlessGuard struct{}

func (g *userlessGuard) Check(ctx *genhttp.Context) bool                { return true }
func (g *userlessGuard) User(ctx *genhttp.Context) auth.Authenticatable { return nil }
func (g *userlessGuard) ID(ctx *genhttp.Context) any                    { return nil }
