package auth_test

import (
	"encoding/json"
	"io"
	nethttp "net/http"
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

type User struct {
	database.Model
	Email    string `db:"email" json:"email"`
	Password string `db:"password" json:"-"`
	ApiToken string `db:"api_token" json:"-"`
}

func (u *User) GetAuthIdentifier() any  { return u.ID }
func (u *User) GetAuthPassword() string { return u.Password }

func setupAuthApp(t *testing.T) (*genhttp.Kernel, *auth.SessionGuard, *auth.TokenGuard) {
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
		created_at TIMESTAMP,
		updated_at TIMESTAMP
	)`)
	require.NoError(t, err)

	hashed, err := hash.Make("secret-password")
	require.NoError(t, err)
	require.NoError(t, database.Create(&User{Email: "alice@example.com", Password: hashed, ApiToken: "token-123"}))

	provider := auth.NewORMUserProvider[User]()
	sessionGuard := auth.NewSessionGuard("web", provider)
	tokenGuard := auth.NewTokenGuard("api", provider)

	app := foundation.New()
	require.NoError(t, app.Boot())
	kernel := genhttp.NewKernel(app, genhttp.KernelConfig{DisableStartupMessage: true})

	// Session middleware (insecure cookie so the httptest client keeps it).
	sessions := session.NewManager(session.Config{
		CookieName:   "test_session",
		CookieSecure: false,
	})
	kernel.Fiber().Use(sessions.Middleware())

	kernel.POST("/login", func(ctx *genhttp.Context) error {
		var body map[string]any
		if err := ctx.JSON(&body); err != nil {
			return err
		}
		user, err := sessionGuard.Attempt(ctx, body)
		if err != nil {
			return ctx.Unauthorized("Invalid credentials")
		}
		return ctx.JSONResponse(map[string]any{"id": user.GetAuthIdentifier()})
	})
	kernel.POST("/logout", func(ctx *genhttp.Context) error {
		if err := sessionGuard.Logout(ctx); err != nil {
			return err
		}
		return ctx.NoContent()
	})
	kernel.GET("/me", func(ctx *genhttp.Context) error {
		user := sessionGuard.User(ctx)
		if user == nil {
			return ctx.Unauthorized()
		}
		return ctx.JSONResponse(map[string]any{"id": user.GetAuthIdentifier()})
	}, auth.Middleware(sessionGuard))
	kernel.GET("/api/me", func(ctx *genhttp.Context) error {
		return ctx.JSONResponse(map[string]any{"id": tokenGuard.ID(ctx)})
	}, auth.Middleware(tokenGuard))

	return kernel, sessionGuard, tokenGuard
}

func cookieHeader(resp *nethttp.Response) string {
	var parts []string
	for _, c := range resp.Cookies() {
		parts = append(parts, c.Name+"="+c.Value)
	}
	return strings.Join(parts, "; ")
}

func TestSessionLoginFlow(t *testing.T) {
	kernel, _, _ := setupAuthApp(t)

	// Unauthenticated request is rejected.
	resp, err := kernel.Fiber().Test(httptest.NewRequest("GET", "/me", nil), -1)
	require.NoError(t, err)
	assert.Equal(t, 401, resp.StatusCode)

	// Wrong password is rejected.
	req := httptest.NewRequest("POST", "/login", strings.NewReader(`{"email":"alice@example.com","password":"wrong"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err = kernel.Fiber().Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, 401, resp.StatusCode)

	// Correct credentials log in and set a session cookie.
	req = httptest.NewRequest("POST", "/login", strings.NewReader(`{"email":"alice@example.com","password":"secret-password"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err = kernel.Fiber().Test(req, -1)
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode)
	cookies := cookieHeader(resp)
	require.NotEmpty(t, cookies)

	// The session cookie authenticates subsequent requests.
	req = httptest.NewRequest("GET", "/me", nil)
	req.Header.Set("Cookie", cookies)
	resp, err = kernel.Fiber().Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	var me map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&me))
	assert.Equal(t, float64(1), me["id"])

	// Logout invalidates the session.
	req = httptest.NewRequest("POST", "/logout", nil)
	req.Header.Set("Cookie", cookies)
	resp, err = kernel.Fiber().Test(req, -1)
	require.NoError(t, err)
	require.Equal(t, 204, resp.StatusCode)

	req = httptest.NewRequest("GET", "/me", nil)
	req.Header.Set("Cookie", cookies)
	resp, err = kernel.Fiber().Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, 401, resp.StatusCode)
}

func TestTokenGuard(t *testing.T) {
	kernel, _, _ := setupAuthApp(t)

	resp, err := kernel.Fiber().Test(httptest.NewRequest("GET", "/api/me", nil), -1)
	require.NoError(t, err)
	assert.Equal(t, 401, resp.StatusCode)

	req := httptest.NewRequest("GET", "/api/me", nil)
	req.Header.Set("Authorization", "Bearer wrong-token")
	resp, err = kernel.Fiber().Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, 401, resp.StatusCode)

	req = httptest.NewRequest("GET", "/api/me", nil)
	req.Header.Set("Authorization", "Bearer token-123")
	resp, err = kernel.Fiber().Test(req, -1)
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	assert.Contains(t, string(body), "1")
}

func TestGate(t *testing.T) {
	gate := auth.NewGate()
	alice := &User{Model: database.Model{ID: 1}}
	bob := &User{Model: database.Model{ID: 2}}

	type post struct{ AuthorID int64 }

	gate.Define("update-post", func(user auth.Authenticatable, args ...any) bool {
		p := args[0].(*post)
		return p.AuthorID == user.GetAuthIdentifier().(int64)
	})

	alicePost := &post{AuthorID: 1}
	assert.True(t, gate.Allows(alice, "update-post", alicePost))
	assert.False(t, gate.Allows(bob, "update-post", alicePost))
	assert.True(t, gate.Denies(bob, "update-post", alicePost))
	assert.False(t, gate.Allows(alice, "undefined-ability"))
	assert.False(t, gate.Allows(nil, "update-post", alicePost))

	require.NoError(t, gate.Authorize(alice, "update-post", alicePost))
	err := gate.Authorize(bob, "update-post", alicePost)
	require.Error(t, err)

	// Before hook: user 2 becomes a superuser.
	gate.Before(func(user auth.Authenticatable, ability string, args ...any) *bool {
		if user != nil && user.GetAuthIdentifier() == int64(2) {
			yes := true
			return &yes
		}
		return nil
	})
	assert.True(t, gate.Allows(bob, "update-post", alicePost))
}
