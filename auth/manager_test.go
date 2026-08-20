package auth_test

import (
	"io"
	"net/http/httptest"
	"testing"

	"github.com/genesysflow/go-genesys/auth"
	"github.com/genesysflow/go-genesys/foundation"
	genhttp "github.com/genesysflow/go-genesys/http"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

func TestManagerGuardRegistration(t *testing.T) {
	manager := auth.NewManager()

	_, err := manager.Guard()
	assert.Error(t, err, "no guards registered")

	provider := auth.NewORMUserProvider[User]()
	web := auth.NewSessionGuard("web", provider)
	api := auth.NewTokenGuard("api", provider)

	manager.RegisterGuard("web", web)
	manager.RegisterGuard("api", api)

	// First registered guard becomes the default.
	guard, err := manager.Guard()
	require.NoError(t, err)
	assert.Same(t, web, guard)

	guard, err = manager.Guard("api")
	require.NoError(t, err)
	assert.Same(t, api, guard)

	manager.SetDefaultGuard("api")
	guard, err = manager.Guard()
	require.NoError(t, err)
	assert.Same(t, api, guard)

	_, err = manager.Guard("missing")
	assert.ErrorContains(t, err, "not registered")
	assert.Panics(t, func() { manager.MustGuard("missing") })
	assert.NotPanics(t, func() { manager.MustGuard("web") })
}

func TestAuthMiddlewareRedirectsBrowsers(t *testing.T) {
	kernel, sessionGuard, _ := setupAuthApp(t)

	kernel.GET("/dashboard", func(ctx *genhttp.Context) error {
		return ctx.String("dash")
	}, auth.Middleware(sessionGuard, auth.MiddlewareOptions{RedirectTo: "/login"}))

	// Browser request: redirected to the login page.
	req := httptest.NewRequest("GET", "/dashboard", nil)
	req.Header.Set("Accept", "text/html")
	resp, err := kernel.Fiber().Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, 302, resp.StatusCode)
	assert.Equal(t, "/login", resp.Header.Get("Location"))

	// API request: still a 401.
	req = httptest.NewRequest("GET", "/dashboard", nil)
	req.Header.Set("Accept", "application/json")
	resp, err = kernel.Fiber().Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, 401, resp.StatusCode)
}

func TestGuestMiddleware(t *testing.T) {
	app := foundation.New()
	require.NoError(t, app.Boot())
	kernel := genhttp.NewKernel(app, genhttp.KernelConfig{DisableStartupMessage: true})

	// A guard that always reports authenticated.
	always := &staticGuard{authenticated: true}
	never := &staticGuard{authenticated: false}

	kernel.GET("/login-auth", func(ctx *genhttp.Context) error {
		return ctx.String("login page")
	}, auth.GuestMiddleware(always, "/home"))
	kernel.GET("/login-guest", func(ctx *genhttp.Context) error {
		return ctx.String("login page")
	}, auth.GuestMiddleware(never, "/home"))

	resp, err := kernel.Fiber().Test(httptest.NewRequest("GET", "/login-auth", nil), -1)
	require.NoError(t, err)
	assert.Equal(t, 302, resp.StatusCode, "authenticated users are redirected away")

	resp, err = kernel.Fiber().Test(httptest.NewRequest("GET", "/login-guest", nil), -1)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	assert.Equal(t, "login page", string(body))
}

func TestTokenGuardQueryParamFallback(t *testing.T) {
	kernel, _, tokenGuard := setupAuthApp(t)
	tokenGuard.QueryParam = "api_token"

	kernel.GET("/query-auth", func(ctx *genhttp.Context) error {
		return ctx.String("ok")
	}, auth.Middleware(tokenGuard))

	resp, err := kernel.Fiber().Test(httptest.NewRequest("GET", "/query-auth?api_token=token-123", nil), -1)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	resp, err = kernel.Fiber().Test(httptest.NewRequest("GET", "/query-auth?api_token=wrong", nil), -1)
	require.NoError(t, err)
	assert.Equal(t, 401, resp.StatusCode)
}

// staticGuard is a Guard with a fixed answer, for middleware tests.
type staticGuard struct{ authenticated bool }

func (g *staticGuard) Check(ctx *genhttp.Context) bool { return g.authenticated }
func (g *staticGuard) User(ctx *genhttp.Context) auth.Authenticatable {
	if g.authenticated {
		return &User{}
	}
	return nil
}
func (g *staticGuard) ID(ctx *genhttp.Context) any { return nil }
