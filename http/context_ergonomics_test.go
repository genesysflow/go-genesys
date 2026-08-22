package http

import (
	"io"
	"net/http/httptest"
	"testing"

	"github.com/genesysflow/go-genesys/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type ergoUser struct {
	ID    int
	Email string
}

func TestContextUserDefaultsToNil(t *testing.T) {
	app := newTestApp()
	router := NewRouter(&mockApplication{}, app)

	router.GET("/me", func(ctx *Context) error {
		assert.Nil(t, ctx.User())
		assert.False(t, ctx.HasUser())
		return ctx.String("ok")
	})

	resp, err := app.Test(httptest.NewRequest("GET", "/me", nil))
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestContextSetUser(t *testing.T) {
	app := newTestApp()
	router := NewRouter(&mockApplication{}, app)

	router.Use(func(ctx *Context, next func() error) error {
		ctx.SetUser(&ergoUser{ID: 7, Email: "ada@example.com"})
		return next()
	})

	router.GET("/me", func(ctx *Context) error {
		require.True(t, ctx.HasUser())
		user, ok := ctx.User().(*ergoUser)
		require.True(t, ok)
		return ctx.String(user.Email)
	})

	resp, err := app.Test(httptest.NewRequest("GET", "/me", nil))
	require.NoError(t, err)
	body, _ := io.ReadAll(resp.Body)
	assert.Equal(t, "ada@example.com", string(body))
}

// UserAs is the typed accessor handlers reach for: it avoids a type
// assertion at every call site.
func TestUserAs(t *testing.T) {
	app := newTestApp()
	router := NewRouter(&mockApplication{}, app)

	router.GET("/typed", func(ctx *Context) error {
		if user, ok := UserAs[ergoUser](ctx); ok {
			return ctx.String("unexpected " + user.Email)
		}
		ctx.SetUser(&ergoUser{ID: 1, Email: "grace@example.com"})
		user, ok := UserAs[ergoUser](ctx)
		require.True(t, ok)
		return ctx.String(user.Email)
	})

	resp, err := app.Test(httptest.NewRequest("GET", "/typed", nil))
	require.NoError(t, err)
	body, _ := io.ReadAll(resp.Body)
	assert.Equal(t, "grace@example.com", string(body))
}

func TestContextSessionNilWithoutMiddleware(t *testing.T) {
	app := newTestApp()
	router := NewRouter(&mockApplication{}, app)

	router.GET("/nosession", func(ctx *Context) error {
		assert.Nil(t, ctx.Session())
		return ctx.String("ok")
	})

	resp, err := app.Test(httptest.NewRequest("GET", "/nosession", nil))
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestContextSessionAndOldInput(t *testing.T) {
	app := newTestApp()
	manager := session.NewManager(session.Config{CookieSecure: false})
	app.Use(manager.Middleware())

	router := NewRouter(&mockApplication{}, app)

	router.POST("/flash", func(ctx *Context) error {
		sess := ctx.Session()
		require.NotNil(t, sess)
		require.NoError(t, sess.FlashInput(map[string]any{"email": "ada@example.com"}))
		return ctx.String("flashed")
	})

	router.GET("/read", func(ctx *Context) error {
		return ctx.String(ctx.Old("email") + "|" + ctx.Old("missing", "fallback"))
	})

	flashResp, err := app.Test(httptest.NewRequest("POST", "/flash", nil))
	require.NoError(t, err)
	require.Equal(t, 200, flashResp.StatusCode)

	cookie := flashResp.Header.Get("Set-Cookie")
	require.NotEmpty(t, cookie, "session cookie should be issued")

	readReq := httptest.NewRequest("GET", "/read", nil)
	readReq.Header.Set("Cookie", cookie)
	readResp, err := app.Test(readReq)
	require.NoError(t, err)

	body, _ := io.ReadAll(readResp.Body)
	assert.Equal(t, "ada@example.com|fallback", string(body))
}

func TestContextRoute(t *testing.T) {
	app := newTestApp()
	router := NewRouter(&mockApplication{}, app)

	router.GET("/users/:user", func(ctx *Context) error {
		route := ctx.Route()
		require.NotNil(t, route)
		assert.Equal(t, "users.show", route.GetName())
		assert.Equal(t, "/users/:user", route.GetPath())
		assert.Equal(t, "GET", route.GetMethod())
		assert.Equal(t, "users.show", ctx.RouteName())
		return ctx.String("ok")
	}).Name("users.show")

	resp, err := app.Test(httptest.NewRequest("GET", "/users/1", nil))
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestContextRouteIs(t *testing.T) {
	app := newTestApp()
	router := NewRouter(&mockApplication{}, app)

	router.GET("/users/:user/edit", func(ctx *Context) error {
		assert.True(t, ctx.RouteIs("users.edit"))
		assert.True(t, ctx.RouteIs("posts.index", "users.*"))
		assert.True(t, ctx.RouteIs("*"))
		assert.False(t, ctx.RouteIs("posts.*"))
		assert.False(t, ctx.RouteIs("users"))
		return ctx.String("ok")
	}).Name("users.edit")

	resp, err := app.Test(httptest.NewRequest("GET", "/users/1/edit", nil))
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestContextRouteIsUnnamedRoute(t *testing.T) {
	app := newTestApp()
	router := NewRouter(&mockApplication{}, app)

	router.GET("/anon", func(ctx *Context) error {
		require.NotNil(t, ctx.Route())
		assert.Equal(t, "", ctx.RouteName())
		assert.False(t, ctx.RouteIs("*"))
		return ctx.String("ok")
	})

	resp, err := app.Test(httptest.NewRequest("GET", "/anon", nil))
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestContextRouteNilForFiberOnlyHandlers(t *testing.T) {
	fiberApp := newTestApp()
	router := NewRouter(&mockApplication{}, fiberApp)

	// A fallback is registered through fiber directly, so no *Route backs it.
	router.Fallback(func(ctx *Context) error {
		assert.Nil(t, ctx.Route())
		assert.Equal(t, "", ctx.RouteName())
		return ctx.Status(404).String("not found")
	})

	resp, err := fiberApp.Test(httptest.NewRequest("GET", "/missing", nil))
	require.NoError(t, err)
	assert.Equal(t, 404, resp.StatusCode)
}

// Route.Middleware() is chained after the route is registered; the
// wrapped handler must still pick it up (an unprotected route otherwise).
func TestRouteMiddlewareAddedAfterRegistrationRuns(t *testing.T) {
	app := newTestApp()
	router := NewRouter(&mockApplication{}, app)

	order := []string{}
	router.GET("/guarded", func(ctx *Context) error {
		order = append(order, "handler")
		return ctx.String("ok")
	}).Middleware(func(ctx *Context, next func() error) error {
		order = append(order, "middleware")
		return next()
	})

	resp, err := app.Test(httptest.NewRequest("GET", "/guarded", nil))
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, []string{"middleware", "handler"}, order)
}

// A route middleware that aborts must stop the handler from running.
func TestRouteMiddlewareAddedAfterRegistrationCanBlock(t *testing.T) {
	app := newTestApp()
	router := NewRouter(&mockApplication{}, app)

	handlerRan := false
	router.GET("/blocked", func(ctx *Context) error {
		handlerRan = true
		return ctx.String("ok")
	}).Middleware(func(ctx *Context, next func() error) error {
		return ctx.Status(403).String("forbidden")
	})

	resp, err := app.Test(httptest.NewRequest("GET", "/blocked", nil))
	require.NoError(t, err)
	assert.Equal(t, 403, resp.StatusCode)
	assert.False(t, handlerRan, "handler must not run when middleware blocks")
}
