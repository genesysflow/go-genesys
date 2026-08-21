package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	genhttp "github.com/genesysflow/go-genesys/http"
	"github.com/genesysflow/go-genesys/http/middleware"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var csrfSecret = []byte("test-secret-at-least-32-bytes-long!!")

// csrfKernel returns a kernel with CSRF protection and a read/write route
// pair. Secure cookies are disabled so the test client (plain HTTP) keeps them.
func csrfKernel(t *testing.T, trusted ...string) *genhttp.Kernel {
	t.Helper()

	k := newKernel(t)
	k.Use(middleware.CSRF(middleware.CSRFConfig{
		Secret:         csrfSecret,
		CookieInsecure: true,
		TrustedOrigins: trusted,
	}))
	k.GET("/form", func(ctx *genhttp.Context) error {
		return ctx.String(middleware.CSRFToken(ctx))
	})
	k.POST("/submit", func(ctx *genhttp.Context) error {
		return ctx.String("accepted")
	})

	return k
}

// issueToken performs the safe-method request that mints a token, returning
// the token and the cookie carrying it.
func issueToken(t *testing.T, k *genhttp.Kernel) (string, *http.Cookie) {
	t.Helper()

	resp, err := k.Fiber().Test(httptest.NewRequest("GET", "/form", nil), -1)
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode)

	cookies := resp.Cookies()
	require.NotEmpty(t, cookies, "a safe request must seed a CSRF cookie")

	var csrfCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == "genesys_csrf" {
			csrfCookie = c
		}
	}
	require.NotNil(t, csrfCookie)

	return csrfCookie.Value, csrfCookie
}

func TestCSRFAcceptsMatchingHeaderToken(t *testing.T) {
	k := csrfKernel(t)
	token, cookie := issueToken(t, k)

	req := httptest.NewRequest("POST", "/submit", nil)
	req.AddCookie(cookie)
	req.Header.Set("X-CSRF-Token", token)

	resp, err := k.Fiber().Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestCSRFAcceptsMatchingFormField(t *testing.T) {
	k := csrfKernel(t)
	token, cookie := issueToken(t, k)

	body := strings.NewReader("_token=" + token + "&comment=hello")
	req := httptest.NewRequest("POST", "/submit", body)
	req.AddCookie(cookie)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := k.Fiber().Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

// TestCSRFRejectsForgedRequests covers the shapes a cross-site attacker can
// actually produce.
func TestCSRFRejectsForgedRequests(t *testing.T) {
	cases := []struct {
		name string
		// build mutates the request to represent the attack.
		build func(t *testing.T, req *http.Request, token string, cookie *http.Cookie)
	}{
		{
			// The canonical CSRF: the browser attaches the cookie
			// automatically, but the attacker cannot read it to supply the
			// out-of-band copy.
			name: "cookie present but no token echoed",
			build: func(_ *testing.T, req *http.Request, _ string, cookie *http.Cookie) {
				req.AddCookie(cookie)
			},
		},
		{
			name:  "no cookie and no token",
			build: func(_ *testing.T, _ *http.Request, _ string, _ *http.Cookie) {},
		},
		{
			name: "token echoed but no cookie",
			build: func(_ *testing.T, req *http.Request, token string, _ *http.Cookie) {
				req.Header.Set("X-CSRF-Token", token)
			},
		},
		{
			name: "echoed token does not match the cookie",
			build: func(_ *testing.T, req *http.Request, _ string, cookie *http.Cookie) {
				req.AddCookie(cookie)
				req.Header.Set("X-CSRF-Token", "some-other-value")
			},
		},
		{
			// An attacker controlling a sibling subdomain can plant a cookie
			// on the parent domain. Signing is what stops them choosing both
			// halves of the double submit.
			name: "attacker-chosen cookie and matching token",
			build: func(_ *testing.T, req *http.Request, _ string, _ *http.Cookie) {
				forged := "attacker-value.attacker-signature"
				req.AddCookie(&http.Cookie{Name: "genesys_csrf", Value: forged})
				req.Header.Set("X-CSRF-Token", forged)
			},
		},
		{
			// Valid token, but the request originates cross-site.
			name: "valid token from an untrusted origin",
			build: func(_ *testing.T, req *http.Request, token string, cookie *http.Cookie) {
				req.AddCookie(cookie)
				req.Header.Set("X-CSRF-Token", token)
				req.Header.Set("Origin", "https://evil.example")
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			k := csrfKernel(t)
			token, cookie := issueToken(t, k)

			req := httptest.NewRequest("POST", "/submit", nil)
			tc.build(t, req, token, cookie)

			resp, err := k.Fiber().Test(req, -1)
			require.NoError(t, err)
			assert.Equal(t, 403, resp.StatusCode)
		})
	}
}

// TestCSRFAllowsTrustedCrossOrigin verifies explicitly trusted origins pass.
func TestCSRFAllowsTrustedCrossOrigin(t *testing.T) {
	k := csrfKernel(t, "https://trusted.example")
	token, cookie := issueToken(t, k)

	req := httptest.NewRequest("POST", "/submit", nil)
	req.AddCookie(cookie)
	req.Header.Set("X-CSRF-Token", token)
	req.Header.Set("Origin", "https://trusted.example")

	resp, err := k.Fiber().Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

// TestCSRFSafeMethodsPassWithoutToken: GET must not require a token, and must
// seed one for the form that follows.
func TestCSRFSafeMethodsPassWithoutToken(t *testing.T) {
	k := csrfKernel(t)

	resp, err := k.Fiber().Test(httptest.NewRequest("GET", "/form", nil), -1)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	token, _ := issueToken(t, k)
	assert.NotEmpty(t, token)
	assert.Contains(t, token, ".", "token must carry its signature")
}

// TestCSRFTokensAreUnpredictable checks tokens are not reused across sessions.
func TestCSRFTokensAreUnpredictable(t *testing.T) {
	k := csrfKernel(t)

	seen := make(map[string]bool)
	for range 25 {
		token, _ := issueToken(t, k)
		require.False(t, seen[token], "CSRF tokens must never repeat")
		seen[token] = true
	}
}

// TestCSRFRequiresSecret: a per-process random fallback would validate in a
// single-instance test and then reject valid tokens behind a load balancer, so
// an unset secret must fail loudly at construction.
func TestCSRFRequiresSecret(t *testing.T) {
	assert.Panics(t, func() {
		middleware.CSRF(middleware.CSRFConfig{})
	})
}

// TestCSRFCookieIsSecureByDefault: Secret is mandatory, so every real caller
// passes a config. A partial config must not silently drop the Secure
// attribute — a CSRF cookie readable off the wire defeats the control.
func TestCSRFCookieIsSecureByDefault(t *testing.T) {
	k := newKernel(t)
	k.Use(middleware.CSRF(middleware.CSRFConfig{Secret: csrfSecret}))
	k.GET("/form", func(ctx *genhttp.Context) error {
		return ctx.String(middleware.CSRFToken(ctx))
	})

	resp, err := k.Fiber().Test(httptest.NewRequest("GET", "/form", nil), -1)
	require.NoError(t, err)

	var csrfCookie *http.Cookie
	for _, c := range resp.Cookies() {
		if c.Name == "genesys_csrf" {
			csrfCookie = c
		}
	}
	require.NotNil(t, csrfCookie, "a partial config must keep the default cookie name")
	assert.True(t, csrfCookie.Secure, "the CSRF cookie must be Secure unless CookieInsecure is set")
	assert.Equal(t, "/", csrfCookie.Path)
}

// TestCSRFCookieInsecureOptsOut confirms local development over http:// can
// still opt out explicitly.
func TestCSRFCookieInsecureOptsOut(t *testing.T) {
	k := csrfKernel(t)

	_, cookie := issueToken(t, k)
	assert.False(t, cookie.Secure)
}

// The http package duplicates CSRFContextKey to share the token with
// views without importing this package. If the key ever drifts, forms
// silently render an empty token and every POST starts failing.
func TestCSRFTokenReachesViewData(t *testing.T) {
	require.Equal(t, "csrf_token", middleware.CSRFContextKey)

	k := csrfKernel(t)
	k.GET("/token-in-data", func(ctx *genhttp.Context) error {
		token, _ := ctx.ViewData()["csrf_token"].(string)
		require.NotEmpty(t, token, "csrf token should be shared with views")
		assert.Equal(t, middleware.CSRFToken(ctx), token)
		return ctx.String("ok")
	}, middleware.CSRF(middleware.CSRFConfig{Secret: csrfSecret, CookieInsecure: true}))

	resp, err := k.Fiber().Test(httptest.NewRequest("GET", "/token-in-data", nil), -1)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}
