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

// A public page still needs to know who is reading it - to greet them,
// to show an edit link, to let a policy see past a draft. ResolveUser
// puts the user on the context without requiring one.
func TestResolveUserMakesTheUserAvailableWithoutRequiringOne(t *testing.T) {
	kernel, _, tokenGuard := setupAuthApp(t)

	kernel.Use(auth.ResolveUser(tokenGuard))
	kernel.GET("/public", func(ctx *genhttp.Context) error {
		if user, ok := genhttp.UserAs[User](ctx); ok {
			return ctx.String("hello " + user.Email)
		}
		return ctx.String("hello guest")
	})

	// Signed in.
	req := httptest.NewRequest("GET", "/public", nil)
	req.Header.Set("Authorization", "Bearer token-123")
	resp, err := kernel.Fiber().Test(req, -1)
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	assert.Equal(t, "hello alice@example.com", string(body))

	// A guest reaches the same page, rather than a 401.
	resp, err = kernel.Fiber().Test(httptest.NewRequest("GET", "/public", nil), -1)
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode)
	body, _ = io.ReadAll(resp.Body)
	assert.Equal(t, "hello guest", string(body))
}

// A user another middleware already resolved is left alone.
func TestResolveUserDoesNotReplaceAnExistingUser(t *testing.T) {
	kernel, _, tokenGuard := setupAuthApp(t)

	kernel.Use(func(ctx *genhttp.Context, next func() error) error {
		ctx.SetUser(&User{Email: "already@example.com"})
		return next()
	})
	kernel.Use(auth.ResolveUser(tokenGuard))
	kernel.GET("/public", func(ctx *genhttp.Context) error {
		user, _ := genhttp.UserAs[User](ctx)
		return ctx.String(user.Email)
	})

	req := httptest.NewRequest("GET", "/public", nil)
	req.Header.Set("Authorization", "Bearer token-123")
	resp, err := kernel.Fiber().Test(req, -1)
	require.NoError(t, err)

	body, _ := io.ReadAll(resp.Body)
	assert.Equal(t, "already@example.com", string(body))
}

// A guest sent to the login page was trying to reach something. The
// middleware records it so the application can send them on afterwards
// - through Intended, which refuses a destination off this host.
func TestMiddlewareRecordsWhereTheGuestWasHeaded(t *testing.T) {
	kernel, _, _ := setupAuthApp(t)

	guard := &alwaysGuestGuard{}
	kernel.GET("/drafts/new", func(ctx *genhttp.Context) error {
		return ctx.String("form")
	}, auth.Middleware(guard, auth.MiddlewareOptions{RedirectTo: "/login"}))

	kernel.GET("/after-login", func(ctx *genhttp.Context) error {
		return ctx.Intended("/").Send()
	})

	tc := genhttp.NewTestCase(t, kernel).WithHeader("Accept", "text/html")
	tc.Get("/drafts/new").AssertRedirect("/login")
	tc.Get("/after-login").AssertRedirect("/drafts/new")
}

// A guest refused with a 401 (an API client) leaves nothing behind.
func TestMiddlewareRecordsNothingWithoutARedirect(t *testing.T) {
	kernel, _, _ := setupAuthApp(t)

	guard := &alwaysGuestGuard{}
	kernel.GET("/api/posts", func(ctx *genhttp.Context) error {
		return ctx.String("posts")
	}, auth.Middleware(guard))
	kernel.GET("/after-login", func(ctx *genhttp.Context) error {
		return ctx.Intended("/home").Send()
	})

	tc := genhttp.NewTestCase(t, kernel)
	tc.Get("/api/posts").AssertUnauthorized()
	tc.Get("/after-login").AssertRedirect("/home")
}

// alwaysGuestGuard refuses everyone, which is what a guest looks like to
// the middleware.
type alwaysGuestGuard struct{}

func (g *alwaysGuestGuard) Check(ctx *genhttp.Context) bool                { return false }
func (g *alwaysGuestGuard) User(ctx *genhttp.Context) auth.Authenticatable { return nil }
func (g *alwaysGuestGuard) ID(ctx *genhttp.Context) any                    { return nil }
