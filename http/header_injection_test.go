package http_test

import (
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/genesysflow/go-genesys/foundation"
	genhttp "github.com/genesysflow/go-genesys/http"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A value that reaches a response header must not be able to end the
// header and start another - the classic response-splitting hole.
func TestHeaderValuesCannotSplitTheResponse(t *testing.T) {
	app := foundation.New()
	require.NoError(t, app.Boot())

	kernel := genhttp.NewKernel(app, genhttp.KernelConfig{DisableStartupMessage: true})
	kernel.GET("/echo", func(ctx *genhttp.Context) error {
		ctx.Response().Header("X-Echo", ctx.Query("v"))
		return ctx.String("ok")
	})

	resp, err := kernel.Fiber().Test(
		httptest.NewRequest("GET", "/echo?v=a%0d%0aSet-Cookie:%20admin=1", nil), -1)
	require.NoError(t, err)

	assert.Empty(t, resp.Header.Get("Set-Cookie"),
		"a header value must not be able to inject another header")
}

// The same for a redirect's Location.
func TestRedirectLocationCannotSplitTheResponse(t *testing.T) {
	app := foundation.New()
	require.NoError(t, app.Boot())

	kernel := genhttp.NewKernel(app, genhttp.KernelConfig{DisableStartupMessage: true})
	kernel.GET("/go", func(ctx *genhttp.Context) error {
		return ctx.RedirectTo(ctx.Query("to")).Send()
	})

	resp, err := kernel.Fiber().Test(
		httptest.NewRequest("GET", "/go?to=/next%0d%0aSet-Cookie:%20admin=1", nil), -1)
	require.NoError(t, err)

	assert.Empty(t, resp.Header.Get("Set-Cookie"))
	assert.NotContains(t, resp.Header.Get("Location"), "\n")
}

// RedirectTo names a target the developer chose, so an external one is
// legitimate - a payment provider, an OAuth endpoint. What must be
// guarded is a target that came from the request, which is what
// Intended is for.
func TestRedirectToAllowsADeveloperChosenExternalTarget(t *testing.T) {
	app := foundation.New()
	require.NoError(t, app.Boot())

	kernel := genhttp.NewKernel(app, genhttp.KernelConfig{DisableStartupMessage: true})
	kernel.GET("/pay", func(ctx *genhttp.Context) error {
		return ctx.RedirectTo("https://payments.example.com/checkout").Send()
	})

	resp, err := kernel.Fiber().Test(httptest.NewRequest("GET", "/pay", nil), -1)
	require.NoError(t, err)
	assert.Equal(t, "https://payments.example.com/checkout", resp.Header.Get("Location"))
}

// Intended sends the visitor where they were headed before they were
// asked to sign in. That destination came from the request, so it is
// only honoured when it stays on this host.
func TestIntendedRefusesToLeaveTheHost(t *testing.T) {
	kernel := testKernel(t)

	kernel.GET("/store", func(ctx *genhttp.Context) error {
		ctx.SetIntendedURL(ctx.Query("to"))
		return ctx.String("stored")
	})
	kernel.GET("/after-login", func(ctx *genhttp.Context) error {
		return ctx.Intended("/dashboard").Send()
	})

	for _, target := range []string{
		"https://evil.example.com/phish",
		"//evil.example.com/phish",
		"/\\evil.example.com",
		"\\/evil.example.com",
		"javascript:alert(1)",
	} {
		tc := genhttp.NewTestCase(t, kernel)
		tc.Get("/store?to=" + url.QueryEscape(target)).AssertOK()

		location := tc.Get("/after-login").Header("Location")
		assert.NotContains(t, location, "evil.example.com",
			"an intended URL of %q must not leave this host", target)
		assert.NotContains(t, location, "javascript:", target)
	}
}

// A path on this host is honoured, which is the whole point.
func TestIntendedHonoursALocalPath(t *testing.T) {
	kernel := testKernel(t)

	kernel.GET("/store", func(ctx *genhttp.Context) error {
		ctx.SetIntendedURL("/drafts/new")
		return ctx.String("stored")
	})
	kernel.GET("/after-login", func(ctx *genhttp.Context) error {
		return ctx.Intended("/dashboard").Send()
	})

	tc := genhttp.NewTestCase(t, kernel)
	tc.Get("/store").AssertOK()
	tc.Get("/after-login").AssertRedirect("/drafts/new")

	// It is spent once: a second login goes to the fallback.
	tc.Get("/after-login").AssertRedirect("/dashboard")
}

// With nothing stored, the fallback is used.
func TestIntendedFallsBack(t *testing.T) {
	kernel := testKernel(t)
	kernel.GET("/after-login", func(ctx *genhttp.Context) error {
		return ctx.Intended("/dashboard").Send()
	})

	genhttp.NewTestCase(t, kernel).Get("/after-login").AssertRedirect("/dashboard")
}
