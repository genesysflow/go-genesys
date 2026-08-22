package http_test

import (
	"io"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/genesysflow/go-genesys/foundation"
	genhttp "github.com/genesysflow/go-genesys/http"
	"github.com/genesysflow/go-genesys/providers"
	"github.com/genesysflow/go-genesys/session"
	"github.com/genesysflow/go-genesys/view"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeViews writes templates into a temp directory and returns its path.
func writeViews(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		path := filepath.Join(dir, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	}
	return dir
}

// viewKernel boots a kernel with views and sessions wired up.
func viewKernel(t *testing.T, viewsDir string) *genhttp.Kernel {
	t.Helper()

	app := foundation.New()
	require.NoError(t, app.Register(&providers.AppServiceProvider{}))
	require.NoError(t, app.Register(&providers.ValidationServiceProvider{}))
	require.NoError(t, app.Register(&providers.ViewServiceProvider{
		Config: &view.Config{Path: viewsDir},
	}))
	require.NoError(t, app.Boot())

	k := genhttp.NewKernel(app, genhttp.KernelConfig{DisableStartupMessage: true})
	k.UseFiber(session.NewManager(session.Config{CookieSecure: false}).Middleware())
	return k
}

// A view rendered after a validation redirect sees the error bag and the
// old input without the handler passing them in.
func TestViewReceivesErrorsAndOldInput(t *testing.T) {
	dir := writeViews(t, map[string]string{
		"users/create.html": `<form>{{if .errors.Has "email"}}<span class="error">{{.errors.First "email"}}</span>{{end}}` +
			`<input name="name" value="{{.old.Get "name"}}"></form>`,
	})

	k := viewKernel(t, dir)
	k.POST("/users", func(ctx *genhttp.Context) error {
		req, err := genhttp.ValidateRequest[storeUserRequest](ctx)
		if err != nil {
			return err
		}
		return ctx.Created(map[string]any{"name": req.Name})
	})
	k.GET("/users/create", func(ctx *genhttp.Context) error {
		return ctx.View("users.create")
	})

	form := url.Values{}
	form.Set("name", "Al")
	form.Set("email", "nope")

	post := httptest.NewRequest("POST", "/users", strings.NewReader(form.Encode()))
	post.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	post.Header.Set("Accept", "text/html")
	post.Header.Set("Referer", "/users/create")

	postResp, err := k.Fiber().Test(post, -1)
	require.NoError(t, err)
	require.Equal(t, 302, postResp.StatusCode)
	cookie := postResp.Header.Get("Set-Cookie")
	require.NotEmpty(t, cookie)

	get := httptest.NewRequest("GET", "/users/create", nil)
	get.Header.Set("Cookie", cookie)
	getResp, err := k.Fiber().Test(get, -1)
	require.NoError(t, err)

	body, _ := io.ReadAll(getResp.Body)
	assert.Contains(t, string(body), `class="error"`)
	assert.Contains(t, string(body), `value="Al"`)
}

// On a clean request the bag is empty but present, so templates that
// reference it do not blow up.
func TestViewErrorsBagPresentWithoutSession(t *testing.T) {
	dir := writeViews(t, map[string]string{
		"plain.html": `errors:{{.errors.Any}} old:{{.old.Get "name" "none"}}`,
	})

	app := foundation.New()
	require.NoError(t, app.Register(&providers.AppServiceProvider{}))
	require.NoError(t, app.Register(&providers.ViewServiceProvider{Config: &view.Config{Path: dir}}))
	require.NoError(t, app.Boot())

	k := genhttp.NewKernel(app, genhttp.KernelConfig{DisableStartupMessage: true})
	k.GET("/plain", func(ctx *genhttp.Context) error {
		return ctx.View("plain")
	})

	resp, err := k.Fiber().Test(httptest.NewRequest("GET", "/plain", nil), -1)
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	assert.Equal(t, "errors:false old:none", string(body))
}

// The authenticated user and flashed session values reach the view too.
func TestViewReceivesUserAndSession(t *testing.T) {
	dir := writeViews(t, map[string]string{
		"dash.html": `{{if .user}}hello {{.user.Email}}{{end}}|{{.session.GetString "status"}}`,
	})

	k := viewKernel(t, dir)
	k.GET("/set-status", func(ctx *genhttp.Context) error {
		require.NoError(t, ctx.Session().Flash("status", "Saved!"))
		return ctx.String("ok")
	})
	k.GET("/dash", func(ctx *genhttp.Context) error {
		ctx.SetUser(&viewUser{Email: "ada@example.com"})
		return ctx.View("dash")
	})

	setResp, err := k.Fiber().Test(httptest.NewRequest("GET", "/set-status", nil), -1)
	require.NoError(t, err)
	cookie := setResp.Header.Get("Set-Cookie")
	require.NotEmpty(t, cookie)

	get := httptest.NewRequest("GET", "/dash", nil)
	get.Header.Set("Cookie", cookie)
	getResp, err := k.Fiber().Test(get, -1)
	require.NoError(t, err)

	body, _ := io.ReadAll(getResp.Body)
	assert.Equal(t, "hello ada@example.com|Saved!", string(body))
}

// Handler-supplied data wins over the shared keys.
func TestViewDataOverridesSharedKeys(t *testing.T) {
	dir := writeViews(t, map[string]string{
		"custom.html": `{{.errors}}`,
	})

	k := viewKernel(t, dir)
	k.GET("/custom", func(ctx *genhttp.Context) error {
		return ctx.View("custom", map[string]any{"errors": "mine"})
	})

	resp, err := k.Fiber().Test(httptest.NewRequest("GET", "/custom", nil), -1)
	require.NoError(t, err)
	body, _ := io.ReadAll(resp.Body)
	assert.Equal(t, "mine", string(body))
}

// The handler's own map must not be mutated by the shared keys: a map
// reused across requests would otherwise leak one request's user into
// the next.
func TestViewDoesNotMutateCallerData(t *testing.T) {
	dir := writeViews(t, map[string]string{
		"shared.html": `{{.title}}`,
	})

	k := viewKernel(t, dir)
	shared := map[string]any{"title": "Dashboard"}

	k.GET("/shared", func(ctx *genhttp.Context) error {
		return ctx.View("shared", shared)
	})

	resp, err := k.Fiber().Test(httptest.NewRequest("GET", "/shared", nil), -1)
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode)

	assert.Equal(t, map[string]any{"title": "Dashboard"}, shared)
}

type viewUser struct{ Email string }

// A page renders inside the application's layout, and a partial can opt
// out of it - an ajax fragment has no business carrying the site chrome.
func TestContextViewUsesTheLayout(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "layouts"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "layouts", "app.html"),
		[]byte(`<html><main>{{.content}}</main></html>`), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(root, "page.html"),
		[]byte(`<p>{{.name}}</p>`), 0o600))

	app := foundation.New()
	require.NoError(t, app.Register(&providers.ViewServiceProvider{
		Config: &view.Config{Path: root, Layout: "layouts.app"},
	}))
	require.NoError(t, app.Boot())

	kernel := genhttp.NewKernel(app, genhttp.KernelConfig{DisableStartupMessage: true})
	kernel.GET("/page", func(ctx *genhttp.Context) error {
		return ctx.View("page", map[string]any{"name": "Ada"})
	})
	kernel.GET("/fragment", func(ctx *genhttp.Context) error {
		return ctx.ViewIn("", "page", map[string]any{"name": "Ada"})
	})

	body := getBody(t, kernel, "/page")
	assert.Contains(t, body, "<html>")
	assert.Contains(t, body, "<p>Ada</p>")

	fragment := getBody(t, kernel, "/fragment")
	assert.Equal(t, "<p>Ada</p>", strings.TrimSpace(fragment))
}

// getBody performs a GET and returns the response body.
func getBody(t *testing.T, kernel *genhttp.Kernel, path string) string {
	t.Helper()

	resp, err := kernel.Fiber().Test(httptest.NewRequest("GET", path, nil), -1)
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return string(body)
}
