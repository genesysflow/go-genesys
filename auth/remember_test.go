package auth_test

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/genesysflow/go-genesys/auth"
	"github.com/genesysflow/go-genesys/database"
	"github.com/genesysflow/go-genesys/foundation"
	"github.com/genesysflow/go-genesys/hash"
	genhttp "github.com/genesysflow/go-genesys/http"
	"github.com/genesysflow/go-genesys/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

// setupRememberApp builds an app whose users table carries a
// remember_token column and whose routes exercise the remember flow.
func setupRememberApp(t *testing.T) (*genhttp.Kernel, *auth.SessionGuard) {
	t.Helper()

	manager := database.NewManager(database.Config{
		Default: "test",
		Connections: map[string]database.ConnectionConfig{
			"test": {Driver: "sqlite", Database: ":memory:"},
		},
	})
	t.Cleanup(func() {
		database.SetDefault(nil)
		manager.Close()
	})
	database.SetDefault(manager)

	_, err := manager.Statement(`CREATE TABLE users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		email TEXT NOT NULL,
		password TEXT NOT NULL,
		api_token TEXT NOT NULL DEFAULT '',
		remember_token TEXT NOT NULL DEFAULT '',
		created_at TIMESTAMP,
		updated_at TIMESTAMP
	)`)
	require.NoError(t, err)

	hashed, err := hash.Make("secret-password")
	require.NoError(t, err)
	require.NoError(t, database.Create(&User{Email: "alice@example.com", Password: hashed}))

	provider := auth.NewORMUserProvider[User]()
	guard := auth.NewSessionGuard("web", provider)
	guard.RememberCookieInsecure = true

	app := foundation.New()
	require.NoError(t, app.Boot())
	kernel := genhttp.NewKernel(app, genhttp.KernelConfig{DisableStartupMessage: true})
	sessions := session.NewManager(session.Config{CookieName: "test_session", CookieSecure: false})
	kernel.Fiber().Use(sessions.Middleware())

	kernel.POST("/login-remember", func(ctx *genhttp.Context) error {
		_, err := guard.AttemptRemember(ctx, map[string]any{
			"email":    "alice@example.com",
			"password": "secret-password",
		})
		if err != nil {
			return ctx.Unauthorized()
		}
		return ctx.String("logged in")
	})
	kernel.GET("/me", func(ctx *genhttp.Context) error {
		if user := guard.User(ctx); user != nil {
			return ctx.String("user:" + user.(*User).Email)
		}
		return ctx.Unauthorized()
	})
	kernel.POST("/logout", func(ctx *genhttp.Context) error {
		require.NoError(t, guard.Logout(ctx))
		return ctx.String("bye")
	})
	return kernel, guard
}

func TestRememberMeSurvivesLostSession(t *testing.T) {
	kernel, _ := setupRememberApp(t)

	resp, err := kernel.Fiber().Test(httptest.NewRequest("POST", "/login-remember", nil), -1)
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode)

	var rememberCookie string
	for _, c := range resp.Cookies() {
		if c.Name == "remember_web" {
			rememberCookie = c.Value
			assert.True(t, c.HttpOnly)
			assert.Greater(t, c.MaxAge, 0)
		}
	}
	require.NotEmpty(t, rememberCookie, "login issued a remember cookie")
	assert.Contains(t, rememberCookie, "|", "cookie carries id|token")

	// Simulate an expired session: send ONLY the remember cookie.
	req := httptest.NewRequest("GET", "/me", nil)
	req.Header.Set("Cookie", "remember_web="+rememberCookie)
	resp, err = kernel.Fiber().Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode, "remember cookie re-authenticates")

	// The database must hold the SHA-256 of the cookie's token half,
	// never the raw token: a leaked users table must not be replayable
	// as remember-me cookies.
	rows, err := database.Default().Table("users").Where("id", 1).Get()
	require.NoError(t, err)
	require.Len(t, rows, 1)
	rawToken := strings.SplitN(rememberCookie, "|", 2)[1]
	sum := sha256.Sum256([]byte(rawToken))
	assert.Equal(t, hex.EncodeToString(sum[:]), rows[0]["remember_token"], "token is hashed at rest")
	assert.NotEqual(t, rawToken, rows[0]["remember_token"])
}

func TestRememberMeRejectsBadCookies(t *testing.T) {
	kernel, _ := setupRememberApp(t)

	send := func(cookie string) int {
		req := httptest.NewRequest("GET", "/me", nil)
		if cookie != "" {
			req.Header.Set("Cookie", "remember_web="+cookie)
		}
		resp, err := kernel.Fiber().Test(req, -1)
		require.NoError(t, err)
		return resp.StatusCode
	}

	assert.Equal(t, 401, send(""), "no cookie, no login")
	assert.Equal(t, 401, send("1|wrong-token"))
	assert.Equal(t, 401, send("999|whatever"))
	assert.Equal(t, 401, send("garbage-without-separator"))
}

func TestLogoutCyclesRememberToken(t *testing.T) {
	kernel, _ := setupRememberApp(t)

	resp, err := kernel.Fiber().Test(httptest.NewRequest("POST", "/login-remember", nil), -1)
	require.NoError(t, err)
	var rememberCookie, sessionCookie string
	for _, c := range resp.Cookies() {
		switch c.Name {
		case "remember_web":
			rememberCookie = c.Value
		case "test_session":
			sessionCookie = c.Value
		}
	}
	require.NotEmpty(t, rememberCookie)

	// Logout with both cookies present.
	req := httptest.NewRequest("POST", "/logout", nil)
	req.Header.Set("Cookie", "test_session="+sessionCookie+"; remember_web="+rememberCookie)
	resp, err = kernel.Fiber().Test(req, -1)
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode)

	// The old remember cookie no longer authenticates: token was cycled.
	req = httptest.NewRequest("GET", "/me", nil)
	req.Header.Set("Cookie", "remember_web="+rememberCookie)
	resp, err = kernel.Fiber().Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, 401, resp.StatusCode)
}
