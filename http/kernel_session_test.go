package http_test

import (
	"net/http/httptest"
	"testing"

	"github.com/genesysflow/go-genesys/foundation"
	genhttp "github.com/genesysflow/go-genesys/http"
	"github.com/genesysflow/go-genesys/providers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// An application that registers the session provider has sessions on its
// requests. Making every application remember to install the middleware
// itself is how a login flow ends up working in tests and failing in
// production.
func TestKernelInstallsTheSessionMiddleware(t *testing.T) {
	app := foundation.New()
	require.NoError(t, app.Register(&providers.AppServiceProvider{}))
	require.NoError(t, app.Register(&providers.SessionServiceProvider{}))
	require.NoError(t, app.Boot())

	kernel := genhttp.NewKernel(app, genhttp.KernelConfig{DisableStartupMessage: true})
	kernel.GET("/session", func(ctx *genhttp.Context) error {
		if ctx.Session() == nil {
			return ctx.Status(500).String("no session")
		}
		return ctx.String("ok")
	})

	resp, err := kernel.Fiber().Test(httptest.NewRequest("GET", "/session", nil), -1)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

// An application that never registered the session provider is
// unaffected, and asking for a session says so rather than panicking.
func TestKernelWithoutASessionProvider(t *testing.T) {
	app := foundation.New()
	require.NoError(t, app.Boot())

	kernel := genhttp.NewKernel(app, genhttp.KernelConfig{DisableStartupMessage: true})
	kernel.GET("/session", func(ctx *genhttp.Context) error {
		assert.Nil(t, ctx.Session())
		return ctx.String("ok")
	})

	resp, err := kernel.Fiber().Test(httptest.NewRequest("GET", "/session", nil), -1)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

// An application that wants to place the session middleware itself - a
// different store per route group, say - turns the automatic one off.
func TestKernelSessionCanBeDisabled(t *testing.T) {
	app := foundation.New()
	require.NoError(t, app.Register(&providers.AppServiceProvider{}))
	require.NoError(t, app.Register(&providers.SessionServiceProvider{}))
	require.NoError(t, app.Boot())

	kernel := genhttp.NewKernel(app, genhttp.KernelConfig{
		DisableStartupMessage: true,
		DisableSession:        true,
	})
	kernel.GET("/session", func(ctx *genhttp.Context) error {
		assert.Nil(t, ctx.Session(), "the kernel should not have installed a session")
		return ctx.String("ok")
	})

	resp, err := kernel.Fiber().Test(httptest.NewRequest("GET", "/session", nil), -1)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}
