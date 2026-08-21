package http

import (
	"encoding/json"
	"io"
	nethttp "net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/genesysflow/go-genesys/errors"
	"github.com/genesysflow/go-genesys/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// withSession builds a router whose fiber app runs session middleware.
func withSession(t *testing.T) *Router {
	t.Helper()
	app := newTestApp()
	manager := session.NewManager(session.Config{CookieSecure: false})
	app.Use(manager.Middleware())
	return NewRouter(&mockApplication{}, app)
}

// readBody decodes a JSON response body into a map.
func readBody(t *testing.T, resp *nethttp.Response) map[string]any {
	t.Helper()
	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(raw, &decoded), "body: %s", raw)
	return decoded
}

// bodyString reads a response body as a string.
func bodyString(t *testing.T, resp *nethttp.Response) string {
	t.Helper()
	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return string(raw)
}

func TestRedirectToSendsFound(t *testing.T) {
	app := newTestApp()
	router := NewRouter(&mockApplication{}, app)

	router.GET("/go", func(ctx *Context) error {
		return ctx.RedirectTo("/dashboard").Send()
	})

	resp, err := app.Test(httptest.NewRequest("GET", "/go", nil))
	require.NoError(t, err)
	assert.Equal(t, 302, resp.StatusCode)
	assert.Equal(t, "/dashboard", resp.Header.Get("Location"))
}

func TestRedirectStatusOverride(t *testing.T) {
	app := newTestApp()
	router := NewRouter(&mockApplication{}, app)

	router.GET("/go", func(ctx *Context) error {
		return ctx.RedirectTo("/dashboard").Status(303).Send()
	})

	resp, err := app.Test(httptest.NewRequest("GET", "/go", nil))
	require.NoError(t, err)
	assert.Equal(t, 303, resp.StatusCode)
}

func TestRedirectBackUsesReferer(t *testing.T) {
	app := newTestApp()
	router := NewRouter(&mockApplication{}, app)

	router.POST("/save", func(ctx *Context) error {
		return ctx.Back("/fallback").Send()
	})

	req := httptest.NewRequest("POST", "/save", nil)
	req.Header.Set("Referer", "/users/create")
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 302, resp.StatusCode)
	assert.Equal(t, "/users/create", resp.Header.Get("Location"))
}

// The Referer is attacker-controlled; an off-host one must not be honoured.
func TestRedirectBackRejectsOffHostReferer(t *testing.T) {
	app := newTestApp()
	router := NewRouter(&mockApplication{}, app)

	router.POST("/save", func(ctx *Context) error {
		return ctx.Back("/fallback").Send()
	})

	req := httptest.NewRequest("POST", "/save", nil)
	req.Header.Set("Referer", "https://evil.example/phish")
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, "/fallback", resp.Header.Get("Location"))
}

func TestRedirectToRouteResolvesNamedRoute(t *testing.T) {
	app := newTestApp()
	router := NewRouter(&mockApplication{}, app)

	router.GET("/users/:user", func(ctx *Context) error {
		return ctx.String("show")
	}).Name("users.show")

	router.POST("/users", func(ctx *Context) error {
		return ctx.RedirectToRoute("users.show", map[string]any{"user": 42}).Send()
	})

	resp, err := app.Test(httptest.NewRequest("POST", "/users", nil))
	require.NoError(t, err)
	assert.Equal(t, 302, resp.StatusCode)
	assert.Equal(t, "/users/42", resp.Header.Get("Location"))
}

func TestRedirectToUnknownRouteErrors(t *testing.T) {
	app := newTestApp()
	router := NewRouter(&mockApplication{}, app)

	router.POST("/users", func(ctx *Context) error {
		err := ctx.RedirectToRoute("does.not.exist").Send()
		require.Error(t, err)
		return ctx.Status(500).String("boom")
	})

	resp, err := app.Test(httptest.NewRequest("POST", "/users", nil))
	require.NoError(t, err)
	assert.Equal(t, 500, resp.StatusCode)
}

func TestRedirectWithErrorsAndInputFlashesForNextRequest(t *testing.T) {
	router := withSession(t)
	app := router.fiber

	router.POST("/users", func(ctx *Context) error {
		return ctx.Back("/users/create").
			WithErrors(map[string][]string{"email": {"The email field is required."}}).
			WithInput().
			With("status", "Please fix the errors below.").
			Send()
	})

	router.GET("/users/create", func(ctx *Context) error {
		bag := ctx.Errors()
		require.NotNil(t, bag)
		return ctx.JSONResponse(map[string]any{
			"hasEmail": bag.Has("email"),
			"first":    bag.First("email"),
			"old":      ctx.Old("email"),
			"oldPass":  ctx.Old("password", "absent"),
			"status":   ctx.Session().Get("status"),
		})
	})

	form := url.Values{}
	form.Set("email", "not-an-email")
	form.Set("password", "hunter2")

	postReq := httptest.NewRequest("POST", "/users", strings.NewReader(form.Encode()))
	postReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	postResp, err := app.Test(postReq)
	require.NoError(t, err)
	require.Equal(t, 302, postResp.StatusCode)

	cookie := postResp.Header.Get("Set-Cookie")
	require.NotEmpty(t, cookie)

	getReq := httptest.NewRequest("GET", "/users/create", nil)
	getReq.Header.Set("Cookie", cookie)
	getResp, err := app.Test(getReq)
	require.NoError(t, err)

	body := readBody(t, getResp)
	assert.Equal(t, true, body["hasEmail"])
	assert.Equal(t, "The email field is required.", body["first"])
	assert.Equal(t, "not-an-email", body["old"])
	assert.Equal(t, "Please fix the errors below.", body["status"])
	// Passwords must never be flashed into session storage.
	assert.Equal(t, "absent", body["oldPass"])
}

// Flash data lives for exactly one request.
func TestFlashedErrorsExpireAfterOneRequest(t *testing.T) {
	router := withSession(t)
	app := router.fiber

	router.POST("/fail", func(ctx *Context) error {
		return ctx.Back("/form").WithErrors(map[string][]string{"name": {"Required."}}).Send()
	})
	router.GET("/form", func(ctx *Context) error {
		return ctx.JSONResponse(map[string]any{"any": ctx.Errors().Any()})
	})

	postResp, err := app.Test(httptest.NewRequest("POST", "/fail", nil))
	require.NoError(t, err)
	cookie := postResp.Header.Get("Set-Cookie")
	require.NotEmpty(t, cookie)

	first := httptest.NewRequest("GET", "/form", nil)
	first.Header.Set("Cookie", cookie)
	firstResp, err := app.Test(first)
	require.NoError(t, err)
	assert.Equal(t, true, readBody(t, firstResp)["any"])

	second := httptest.NewRequest("GET", "/form", nil)
	second.Header.Set("Cookie", cookie)
	secondResp, err := app.Test(second)
	require.NoError(t, err)
	assert.Equal(t, false, readBody(t, secondResp)["any"], "flashed errors must not survive a second request")
}

func TestRedirectWithErrorsAcceptsValidationError(t *testing.T) {
	router := withSession(t)
	app := router.fiber

	router.POST("/users", func(ctx *Context) error {
		validationErr := errors.NewValidationError(map[string][]string{"name": {"The name field is required."}})
		return ctx.Back("/form").WithErrors(validationErr).Send()
	})
	router.GET("/form", func(ctx *Context) error {
		return ctx.String(ctx.Errors().First("name"))
	})

	postResp, err := app.Test(httptest.NewRequest("POST", "/users", nil))
	require.NoError(t, err)
	cookie := postResp.Header.Get("Set-Cookie")

	getReq := httptest.NewRequest("GET", "/form", nil)
	getReq.Header.Set("Cookie", cookie)
	getResp, err := app.Test(getReq)
	require.NoError(t, err)
	assert.Equal(t, "The name field is required.", bodyString(t, getResp))
}

func TestRedirectWithErrorsAcceptsPlainError(t *testing.T) {
	router := withSession(t)
	app := router.fiber

	router.POST("/users", func(ctx *Context) error {
		return ctx.Back("/form").WithErrors(errors.BadRequest("Something went wrong.")).Send()
	})
	router.GET("/form", func(ctx *Context) error {
		return ctx.String(ctx.Errors().First())
	})

	postResp, err := app.Test(httptest.NewRequest("POST", "/users", nil))
	require.NoError(t, err)
	cookie := postResp.Header.Get("Set-Cookie")

	getReq := httptest.NewRequest("GET", "/form", nil)
	getReq.Header.Set("Cookie", cookie)
	getResp, err := app.Test(getReq)
	require.NoError(t, err)
	assert.Equal(t, "Something went wrong.", bodyString(t, getResp))
}

// WithInput(explicit) overrides the request payload.
func TestRedirectWithExplicitInput(t *testing.T) {
	router := withSession(t)
	app := router.fiber

	router.POST("/users", func(ctx *Context) error {
		return ctx.Back("/form").WithInput(map[string]any{"nickname": "ada"}).Send()
	})
	router.GET("/form", func(ctx *Context) error {
		return ctx.String(ctx.Old("nickname"))
	})

	postResp, err := app.Test(httptest.NewRequest("POST", "/users", nil))
	require.NoError(t, err)
	cookie := postResp.Header.Get("Set-Cookie")

	getReq := httptest.NewRequest("GET", "/form", nil)
	getReq.Header.Set("Cookie", cookie)
	getResp, err := app.Test(getReq)
	require.NoError(t, err)
	assert.Equal(t, "ada", bodyString(t, getResp))
}

// Without session middleware the redirect still happens; flashes are
// simply dropped rather than panicking.
func TestRedirectWithoutSessionStillRedirects(t *testing.T) {
	app := newTestApp()
	router := NewRouter(&mockApplication{}, app)

	router.POST("/users", func(ctx *Context) error {
		return ctx.Back("/form").
			WithErrors(map[string][]string{"name": {"Required."}}).
			WithInput().
			Send()
	})

	resp, err := app.Test(httptest.NewRequest("POST", "/users", nil))
	require.NoError(t, err)
	assert.Equal(t, 302, resp.StatusCode)
	assert.Equal(t, "/form", resp.Header.Get("Location"))
}

// ctx.Errors() is never nil, so templates and handlers can call straight
// into it without a guard.
func TestErrorsBagIsNeverNil(t *testing.T) {
	app := newTestApp()
	router := NewRouter(&mockApplication{}, app)

	router.GET("/plain", func(ctx *Context) error {
		bag := ctx.Errors()
		require.NotNil(t, bag)
		assert.False(t, bag.Any())
		return ctx.String("ok")
	})

	resp, err := app.Test(httptest.NewRequest("GET", "/plain", nil))
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}
