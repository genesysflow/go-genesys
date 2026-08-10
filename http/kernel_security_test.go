package http_test

import (
	"errors"
	"fmt"
	"io"
	"net/http/httptest"
	"testing"

	"github.com/genesysflow/go-genesys/contracts"
	"github.com/genesysflow/go-genesys/foundation"
	genhttp "github.com/genesysflow/go-genesys/http"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// bootKernel builds a kernel without the AppServiceProvider, so the fallback
// error handler in the kernel itself is the one under test.
func bootKernel(t *testing.T) *genhttp.Kernel {
	t.Helper()

	app := foundation.New()
	require.NoError(t, app.Boot())

	return genhttp.NewKernel(app, genhttp.KernelConfig{DisableStartupMessage: true})
}

// TestErrorHandlerDoesNotLeakInternalErrors is the regression test for
// information disclosure.
//
// A bare error's text routinely carries SQL fragments, filesystem paths and
// upstream hostnames. It must be logged, not returned, unless debug mode is
// explicitly on.
func TestErrorHandlerDoesNotLeakInternalErrors(t *testing.T) {
	t.Setenv("APP_DEBUG", "false")

	k := bootKernel(t)
	k.GET("/boom", func(ctx *genhttp.Context) error {
		return errors.New(`pq: relation "internal_secrets" does not exist at /srv/app/db.go:42`)
	})

	req := httptest.NewRequest("GET", "/boom", nil)
	req.Header.Set("Accept", "application/json")

	resp, err := k.Fiber().Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, 500, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	assert.NotContains(t, string(body), "internal_secrets")
	assert.NotContains(t, string(body), "/srv/app/db.go")
	assert.NotContains(t, string(body), "pq:")
	assert.Contains(t, string(body), "Internal Server Error")
}

// TestErrorHandlerKeepsDeliberateMessages: an error that deliberately carries
// a client-facing message must still deliver it.
func TestErrorHandlerKeepsDeliberateMessages(t *testing.T) {
	t.Setenv("APP_DEBUG", "false")

	k := bootKernel(t)
	k.GET("/missing", func(ctx *genhttp.Context) error {
		return contracts.NewHTTPError(404, "Article not found")
	})

	req := httptest.NewRequest("GET", "/missing", nil)
	req.Header.Set("Accept", "application/json")

	resp, err := k.Fiber().Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, 404, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	assert.Contains(t, string(body), "Article not found")
}

// TestErrorHandlerClassifiesWrappedErrors: wrapping must not collapse an
// intended status code to a generic 500.
func TestErrorHandlerClassifiesWrappedErrors(t *testing.T) {
	t.Setenv("APP_DEBUG", "false")

	k := bootKernel(t)
	k.GET("/wrapped", func(ctx *genhttp.Context) error {
		return fmt.Errorf("while loading article: %w", contracts.NewHTTPError(404, "Article not found"))
	})

	req := httptest.NewRequest("GET", "/wrapped", nil)
	req.Header.Set("Accept", "application/json")

	resp, err := k.Fiber().Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, 404, resp.StatusCode)
}

// TestDebugDefaultsOff is the regression test for the default that made the
// leak above reachable: debug mode had to be explicitly disabled rather than
// explicitly enabled, so any deployment that simply did not set APP_DEBUG
// served stack traces to clients.
func TestDebugDefaultsOff(t *testing.T) {
	t.Setenv("APP_DEBUG", "")

	app := foundation.New()
	assert.False(t, app.IsDebug(),
		"APP_DEBUG must default to off; debug mode discloses internals to clients")
}

// TestDebugCanBeEnabledExplicitly confirms the opt-in still works.
func TestDebugCanBeEnabledExplicitly(t *testing.T) {
	t.Setenv("APP_DEBUG", "true")

	app := foundation.New()
	assert.True(t, app.IsDebug())
}
