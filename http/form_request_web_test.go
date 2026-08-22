package http_test

import (
	stderrors "errors"
	"io"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/genesysflow/go-genesys/foundation"
	genhttp "github.com/genesysflow/go-genesys/http"
	"github.com/genesysflow/go-genesys/providers"
	"github.com/genesysflow/go-genesys/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// webKernel boots a kernel with session middleware, the shape a
// browser-facing app has.
func webKernel(t *testing.T) *genhttp.Kernel {
	t.Helper()

	app := foundation.New()
	require.NoError(t, app.Register(&providers.AppServiceProvider{}))
	require.NoError(t, app.Register(&providers.ValidationServiceProvider{}))
	require.NoError(t, app.Boot())

	k := genhttp.NewKernel(app, genhttp.KernelConfig{DisableStartupMessage: true})
	k.UseFiber(session.NewManager(session.Config{CookieSecure: false}).Middleware())

	k.POST("/users", func(ctx *genhttp.Context) error {
		req, err := genhttp.ValidateRequest[storeUserRequest](ctx)
		if err != nil {
			return err
		}
		return ctx.Created(map[string]any{"name": req.Name})
	})

	k.GET("/users/create", func(ctx *genhttp.Context) error {
		return ctx.JSONResponse(map[string]any{
			"emailError": ctx.Errors().First("email"),
			"anyErrors":  ctx.Errors().Any(),
			"oldName":    ctx.Old("name"),
		})
	})

	return k
}

func postInvalidForm(t *testing.T, k *genhttp.Kernel, headers map[string]string) (int, string, string) {
	t.Helper()

	form := url.Values{}
	form.Set("name", "Al")
	form.Set("email", "not-an-email")

	req := httptest.NewRequest("POST", "/users", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Referer", "/users/create")
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	resp, err := k.Fiber().Test(req, -1)
	require.NoError(t, err)

	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body), resp.Header.Get("Set-Cookie")
}

// A browser posting a form gets Laravel's redirect-back-with-errors, not
// a 422 JSON blob it cannot render.
func TestValidationFailureRedirectsBrowserRequests(t *testing.T) {
	k := webKernel(t)

	status, _, cookie := postInvalidForm(t, k, map[string]string{"Accept": "text/html"})
	require.Equal(t, 302, status)
	require.NotEmpty(t, cookie)

	follow := httptest.NewRequest("GET", "/users/create", nil)
	follow.Header.Set("Cookie", cookie)
	followResp, err := k.Fiber().Test(follow, -1)
	require.NoError(t, err)

	body, _ := io.ReadAll(followResp.Body)
	assert.Contains(t, string(body), `"anyErrors":true`)
	assert.Contains(t, string(body), "email")
	// Old input repopulates the form.
	assert.Contains(t, string(body), `"oldName":"Al"`)
}

// The redirect goes back to the form the browser came from.
func TestValidationRedirectTargetsReferer(t *testing.T) {
	k := webKernel(t)

	form := url.Values{}
	form.Set("name", "Al")

	req := httptest.NewRequest("POST", "/users", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "text/html")
	req.Header.Set("Referer", "/users/create")

	resp, err := k.Fiber().Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, 302, resp.StatusCode)
	assert.Equal(t, "/users/create", resp.Header.Get("Location"))
}

// API clients keep the 422 envelope.
func TestValidationFailureStillReturnsJSONForAPIClients(t *testing.T) {
	k := webKernel(t)

	status, body, _ := postInvalidForm(t, k, map[string]string{"Accept": "application/json"})
	assert.Equal(t, 422, status)
	assert.Contains(t, body, `"errors"`)
	assert.Contains(t, body, "message")
}

// XHR from a page that accepts HTML must still get JSON.
func TestValidationFailureReturnsJSONForAjax(t *testing.T) {
	k := webKernel(t)

	status, body, _ := postInvalidForm(t, k, map[string]string{
		"Accept":           "text/html",
		"X-Requested-With": "XMLHttpRequest",
	})
	assert.Equal(t, 422, status)
	assert.Contains(t, body, `"errors"`)
}

// Without a session there is nowhere to flash errors, so an API-shaped
// 422 is the only correct answer - never a redirect that loses them.
func TestValidationFailureFallsBackToJSONWithoutSession(t *testing.T) {
	k := formKernel(t) // no session middleware

	form := url.Values{}
	form.Set("name", "Al")

	req := httptest.NewRequest("POST", "/users", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "text/html")
	req.Header.Set("Referer", "/users/create")

	resp, err := k.Fiber().Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, 422, resp.StatusCode)
}

// Session data flashed while handling a request must survive the handler
// returning an error - otherwise the redirect above arrives with an empty
// error bag.
func TestSessionIsSavedWhenHandlerReturnsError(t *testing.T) {
	app := foundation.New()
	require.NoError(t, app.Register(&providers.AppServiceProvider{}))
	require.NoError(t, app.Boot())

	k := genhttp.NewKernel(app, genhttp.KernelConfig{DisableStartupMessage: true})
	k.UseFiber(session.NewManager(session.Config{CookieSecure: false}).Middleware())

	k.GET("/flash-then-fail", func(ctx *genhttp.Context) error {
		require.NoError(t, ctx.Session().Set("marker", "kept"))
		return stderrors.New("handler failed")
	})
	k.GET("/read-marker", func(ctx *genhttp.Context) error {
		value, _ := ctx.Session().Get("marker").(string)
		return ctx.String(value)
	})

	failResp, err := k.Fiber().Test(httptest.NewRequest("GET", "/flash-then-fail", nil), -1)
	require.NoError(t, err)
	cookie := failResp.Header.Get("Set-Cookie")
	require.NotEmpty(t, cookie, "session cookie should be issued even when the handler fails")

	read := httptest.NewRequest("GET", "/read-marker", nil)
	read.Header.Set("Cookie", cookie)
	readResp, err := k.Fiber().Test(read, -1)
	require.NoError(t, err)

	body, _ := io.ReadAll(readResp.Body)
	assert.Equal(t, "kept", string(body))
}
