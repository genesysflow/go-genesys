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

// TestRedirectBackRefusesOffsiteReferer is the regression test for an open
// redirect.
//
// The Referer header is attacker-controlled. Following it unconditionally lets
// an attacker send a victim to a legitimate URL on this site and have the site
// bounce them onward to a destination of the attacker's choosing, lending the
// application's domain to a phishing page.
func TestRedirectBackRefusesOffsiteReferer(t *testing.T) {
	app := foundation.New()
	require.NoError(t, app.Register(&providers.AppServiceProvider{}))
	require.NoError(t, app.Boot())

	k := genhttp.NewKernel(app, genhttp.KernelConfig{DisableStartupMessage: true})
	k.GET("/back", func(ctx *genhttp.Context) error {
		return ctx.RedirectBack("/fallback")
	})

	cases := []struct {
		name     string
		referer  string
		wantLoc  string
		followed bool
	}{
		{"absolute offsite URL", "https://evil.example/phish", "/fallback", false},
		{"protocol-relative URL", "//evil.example/phish", "/fallback", false},
		{"javascript scheme", "javascript:alert(1)", "/fallback", false},
		{"data scheme", "data:text/html,<script>alert(1)</script>", "/fallback", false},
		{"offsite with matching prefix", "https://example.com.evil.example/x", "/fallback", false},
		{"backslash protocol-relative", `/\evil.example/phish`, "/fallback", false},
		{"double backslash", `\\evil.example/phish`, "/fallback", false},
		{"no referer at all", "", "/fallback", false},
		{"same-host relative path", "/dashboard", "/dashboard", true},
		{"same-host absolute URL", "http://example.com/dashboard", "http://example.com/dashboard", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "http://example.com/back", nil)
			req.Host = "example.com"
			if tc.referer != "" {
				req.Header.Set("Referer", tc.referer)
			}

			resp, err := k.Fiber().Test(req, -1)
			require.NoError(t, err)

			location := resp.Header.Get("Location")
			assert.Equal(t, tc.wantLoc, location)

			if !tc.followed {
				assert.NotContains(t, location, "evil.example",
					"must never redirect to an attacker-controlled host")
			}
		})
	}
}

// TestRedirectBackDefaultsToRoot checks the no-fallback form.
func TestRedirectBackDefaultsToRoot(t *testing.T) {
	app := foundation.New()
	require.NoError(t, app.Register(&providers.AppServiceProvider{}))
	require.NoError(t, app.Boot())

	k := genhttp.NewKernel(app, genhttp.KernelConfig{DisableStartupMessage: true})
	k.GET("/back", func(ctx *genhttp.Context) error {
		return ctx.RedirectBack()
	})

	req := httptest.NewRequest("GET", "http://example.com/back", nil)
	req.Host = "example.com"
	req.Header.Set("Referer", "https://evil.example/phish")

	resp, err := k.Fiber().Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, "/", resp.Header.Get("Location"))
}
