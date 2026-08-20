package http_test

import (
	"io"
	"net/http/httptest"
	"testing"

	"github.com/genesysflow/go-genesys/database"
	"github.com/genesysflow/go-genesys/foundation"
	genhttp "github.com/genesysflow/go-genesys/http"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

func TestFallback(t *testing.T) {
	k := bootKernel(t)
	k.GET("/exists", func(ctx *genhttp.Context) error { return ctx.String("yes") })
	k.Router().Fallback(func(ctx *genhttp.Context) error {
		return ctx.Status(404).JSONResponse(map[string]any{"message": "custom not found"})
	})

	resp, err := k.Fiber().Test(httptest.NewRequest("GET", "/exists", nil), -1)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	resp, err = k.Fiber().Test(httptest.NewRequest("GET", "/missing", nil), -1)
	require.NoError(t, err)
	assert.Equal(t, 404, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	assert.Contains(t, string(body), "custom not found")
}

func TestRedirectRoute(t *testing.T) {
	k := bootKernel(t)
	k.Router().Redirect("/old", "/new", 301)

	resp, err := k.Fiber().Test(httptest.NewRequest("GET", "/old", nil), -1)
	require.NoError(t, err)
	assert.Equal(t, 301, resp.StatusCode)
	assert.Equal(t, "/new", resp.Header.Get("Location"))
}

func TestMiddlewareGroupsAndAliases(t *testing.T) {
	k := bootKernel(t)

	var order []string
	mark := func(label string) genhttp.MiddlewareFunc {
		return func(ctx *genhttp.Context, next func() error) error {
			order = append(order, label)
			return next()
		}
	}

	k.DefineMiddlewareGroup("web", mark("session"), mark("csrf"))
	k.AliasMiddleware("auth", mark("auth"))

	k.GET("/page", func(ctx *genhttp.Context) error { return ctx.String("ok") },
		append(k.MiddlewareGroup("web"), k.Named("auth")...)...)

	resp, err := k.Fiber().Test(httptest.NewRequest("GET", "/page", nil), -1)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, []string{"session", "csrf", "auth"}, order)

	assert.Panics(t, func() { k.MiddlewareGroup("nope") })
	assert.Panics(t, func() { k.Named("nope") })
}

type bindableUser struct {
	database.Model
	Name string `db:"name"`
	Slug string `db:"slug"`
}

func (bindableUser) TableName() string { return "bindable_users" }

func TestBindModel(t *testing.T) {
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
	_, err := manager.Statement(`CREATE TABLE bindable_users (
		id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT, slug TEXT,
		created_at TIMESTAMP, updated_at TIMESTAMP)`)
	require.NoError(t, err)
	require.NoError(t, database.Create(&bindableUser{Name: "Alice", Slug: "alice"}))

	app := foundation.New()
	require.NoError(t, app.Boot())
	k := genhttp.NewKernel(app, genhttp.KernelConfig{DisableStartupMessage: true})

	k.GET("/users/:user", func(ctx *genhttp.Context) error {
		user, err := genhttp.BindModel[bindableUser](ctx, "user")
		if err != nil {
			return err
		}
		return ctx.String(user.Name)
	})
	k.GET("/by-slug/:slug", func(ctx *genhttp.Context) error {
		user, err := genhttp.BindModelBy[bindableUser](ctx, "slug", "slug")
		if err != nil {
			return err
		}
		return ctx.String(user.Name)
	})

	resp, err := k.Fiber().Test(httptest.NewRequest("GET", "/users/1", nil), -1)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	assert.Equal(t, "Alice", string(body))

	resp, err = k.Fiber().Test(httptest.NewRequest("GET", "/users/999", nil), -1)
	require.NoError(t, err)
	assert.Equal(t, 404, resp.StatusCode)

	resp, err = k.Fiber().Test(httptest.NewRequest("GET", "/by-slug/alice", nil), -1)
	require.NoError(t, err)
	body, _ = io.ReadAll(resp.Body)
	assert.Equal(t, "Alice", string(body))
}
