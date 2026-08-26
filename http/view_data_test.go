package http

import (
	"io"
	"net/http/httptest"
	"testing"

	"github.com/genesysflow/go-genesys/session"
	"github.com/genesysflow/go-genesys/support"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- ViewData ---

// ViewData exposes the per-request values every template can rely on,
// even when the handler passes nothing of its own.
func TestContextViewDataSharesRequestValues(t *testing.T) {
	runHandler(t, func(ctx *Context) error {
		data := ctx.ViewData()

		assert.Contains(t, data, "errors")
		assert.Contains(t, data, "old")
		assert.Contains(t, data, "session")
		assert.Contains(t, data, "user")
		assert.Contains(t, data, "gate")

		assert.IsType(t, &support.MessageBag{}, data["errors"])
		assert.IsType(t, OldInput{}, data["old"])
		assert.Nil(t, data["user"], "a guest request shares a nil user")
		assert.IsType(t, denyAll{}, data["gate"])
		assert.NotContains(t, data, csrfContextKey, "no CSRF token unless the middleware set one")

		return ctx.String("ok")
	})
}

// The handler's own keys are layered on top, and win where they collide.
func TestContextViewDataHandlerKeysWin(t *testing.T) {
	runHandler(t, func(ctx *Context) error {
		ctx.SetUser("the real user")

		data := ctx.ViewData(map[string]any{
			"title": "Posts",
			"user":  "handler override",
		})

		assert.Equal(t, "Posts", data["title"])
		assert.Equal(t, "handler override", data["user"])
		assert.Contains(t, data, "errors", "shared keys survive alongside")

		return ctx.String("ok")
	})
}

// The handler's map is never mutated: one reused across requests would
// otherwise leak one request's user into the next.
func TestContextViewDataDoesNotMutateCallerMap(t *testing.T) {
	shared := map[string]any{"title": "Posts"}

	runHandler(t, func(ctx *Context) error {
		ctx.ViewData(shared)
		return ctx.String("ok")
	})

	assert.Equal(t, map[string]any{"title": "Posts"}, shared)
}

// A CSRF token placed in the context store by middleware reaches the
// template, which is what lets a form render its hidden field.
func TestContextViewDataIncludesCSRFToken(t *testing.T) {
	runHandler(t, func(ctx *Context) error {
		ctx.Set(csrfContextKey, "tok-123")

		data := ctx.ViewData()
		assert.Equal(t, "tok-123", data[csrfContextKey])

		return ctx.String("ok")
	})
}

// A non-string value under the CSRF key is not shared, so a template
// cannot render something that is not a token.
func TestContextViewDataIgnoresNonStringCSRFToken(t *testing.T) {
	runHandler(t, func(ctx *Context) error {
		ctx.Set(csrfContextKey, 12345)
		assert.NotContains(t, ctx.ViewData(), csrfContextKey)
		return ctx.String("ok")
	})
}

// Only the first map is read, matching every other variadic helper here.
func TestContextViewDataUsesFirstMapOnly(t *testing.T) {
	runHandler(t, func(ctx *Context) error {
		data := ctx.ViewData(
			map[string]any{"title": "First"},
			map[string]any{"title": "Second", "extra": true},
		)

		assert.Equal(t, "First", data["title"])
		assert.NotContains(t, data, "extra")

		return ctx.String("ok")
	})
}

// --- OldInput ---

// Without a session there is nothing flashed, and asking must not panic.
func TestOldInputWithoutSession(t *testing.T) {
	old := OldInput{}

	assert.False(t, old.Has("email"))
	assert.False(t, old.Any())
	assert.Empty(t, old.Get("email"))
	assert.Equal(t, "fallback", old.Get("email", "fallback"))
}

// The same holds for the OldInput a session-less request shares with its
// templates.
func TestContextViewDataOldInputWithoutSession(t *testing.T) {
	runHandler(t, func(ctx *Context) error {
		old, ok := ctx.ViewData()["old"].(OldInput)
		require.True(t, ok)

		assert.False(t, old.Any())
		assert.False(t, old.Has("email"))
		assert.Equal(t, "-", old.Get("email", "-"))

		return ctx.String("ok")
	})
}

// With input flashed by the previous request, Has answers by key rather
// than by comparing values, so a field flashed as "" still reports
// present - which is what lets a form redisplay a cleared field.
func TestOldInputHasAndAnyAfterFlash(t *testing.T) {
	app := newTestApp()
	app.Use(session.NewManager(session.Config{CookieSecure: false}).Middleware())
	router := NewRouter(&mockApplication{}, app)

	router.POST("/flash", func(ctx *Context) error {
		sess := ctx.Session()
		require.NotNil(t, sess)
		require.NoError(t, sess.FlashInput(map[string]any{
			"email":    "ada@example.com",
			"nickname": "",
		}))
		return ctx.String("flashed")
	})

	router.GET("/form", func(ctx *Context) error {
		old, ok := ctx.ViewData()["old"].(OldInput)
		require.True(t, ok)

		assert.True(t, old.Any(), "the previous request flashed input")
		assert.True(t, old.Has("email"))
		assert.True(t, old.Has("nickname"), "a field flashed empty is still present")
		assert.False(t, old.Has("never-sent"))
		assert.Equal(t, "ada@example.com", old.Get("email"))
		assert.Equal(t, "fallback", old.Get("never-sent", "fallback"))

		return ctx.String("read")
	})

	flashResp, err := app.Test(httptest.NewRequest("POST", "/flash", nil), -1)
	require.NoError(t, err)
	require.Equal(t, 200, flashResp.StatusCode)

	cookie := flashResp.Header.Get("Set-Cookie")
	require.NotEmpty(t, cookie)

	readReq := httptest.NewRequest("GET", "/form", nil)
	readReq.Header.Set("Cookie", cookie)
	readResp, err := app.Test(readReq, -1)
	require.NoError(t, err)

	body, _ := io.ReadAll(readResp.Body)
	assert.Equal(t, "read", string(body))
}

// A session that had nothing flashed into it reports no old input.
func TestOldInputAnyWithEmptySession(t *testing.T) {
	app := newTestApp()
	app.Use(session.NewManager(session.Config{CookieSecure: false}).Middleware())
	router := NewRouter(&mockApplication{}, app)

	router.GET("/form", func(ctx *Context) error {
		require.NotNil(t, ctx.Session())

		old, ok := ctx.ViewData()["old"].(OldInput)
		require.True(t, ok)
		assert.False(t, old.Any())
		assert.False(t, old.Has("email"))

		return ctx.String("ok")
	})

	resp, err := app.Test(httptest.NewRequest("GET", "/form", nil), -1)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}
