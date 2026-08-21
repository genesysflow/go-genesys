package errors_test

import (
	"io"
	"net/http/httptest"
	"testing"

	goerrors "errors"

	frameworkerrors "github.com/genesysflow/go-genesys/errors"
	"github.com/genesysflow/go-genesys/foundation"
	genhttp "github.com/genesysflow/go-genesys/http"
	"github.com/genesysflow/go-genesys/providers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func errorKernel(t *testing.T) *genhttp.Kernel {
	t.Helper()
	app := foundation.New()
	require.NoError(t, app.Register(&providers.AppServiceProvider{}))
	require.NoError(t, app.Boot())
	k := genhttp.NewKernel(app, genhttp.KernelConfig{DisableStartupMessage: true})
	k.GET("/missing", func(ctx *genhttp.Context) error {
		return frameworkerrors.NotFound("Page not found")
	})
	k.GET("/boom", func(ctx *genhttp.Context) error {
		return goerrors.New("secret internal detail")
	})
	return k
}

func TestBrowserRequestsGetHTMLErrorPage(t *testing.T) {
	t.Setenv("APP_DEBUG", "false")
	k := errorKernel(t)

	req := httptest.NewRequest("GET", "/missing", nil)
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	resp, err := k.Fiber().Test(req, -1)
	require.NoError(t, err)

	assert.Equal(t, 404, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("Content-Type"), "text/html")
	body, _ := io.ReadAll(resp.Body)
	assert.Contains(t, string(body), "404")
	assert.Contains(t, string(body), "Page not found")
}

func TestAPIRequestsStillGetJSON(t *testing.T) {
	t.Setenv("APP_DEBUG", "false")
	k := errorKernel(t)

	req := httptest.NewRequest("GET", "/missing", nil)
	req.Header.Set("Accept", "application/json")
	resp, err := k.Fiber().Test(req, -1)
	require.NoError(t, err)

	assert.Equal(t, 404, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("Content-Type"), "application/json")
}

func TestHTMLPageHidesInternalsWithoutDebug(t *testing.T) {
	t.Setenv("APP_DEBUG", "false")
	k := errorKernel(t)

	req := httptest.NewRequest("GET", "/boom", nil)
	req.Header.Set("Accept", "text/html")
	resp, err := k.Fiber().Test(req, -1)
	require.NoError(t, err)

	assert.Equal(t, 500, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	assert.NotContains(t, string(body), "secret internal detail")
}

func TestHTMLDebugPageShowsStack(t *testing.T) {
	t.Setenv("APP_DEBUG", "true")
	k := errorKernel(t)

	req := httptest.NewRequest("GET", "/boom", nil)
	req.Header.Set("Accept", "text/html")
	resp, err := k.Fiber().Test(req, -1)
	require.NoError(t, err)

	assert.Equal(t, 500, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	assert.Contains(t, string(body), "secret internal detail")
	assert.Contains(t, string(body), "goroutine")
}
