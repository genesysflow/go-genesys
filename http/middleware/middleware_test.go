package middleware_test

import (
	"encoding/base64"
	"io"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/genesysflow/go-genesys/foundation"
	genhttp "github.com/genesysflow/go-genesys/http"
	"github.com/genesysflow/go-genesys/http/middleware"
	"github.com/genesysflow/go-genesys/providers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newKernel builds a booted kernel for middleware tests.
func newKernel(t *testing.T) *genhttp.Kernel {
	t.Helper()

	app := foundation.New()
	require.NoError(t, app.Register(&providers.AppServiceProvider{}))
	require.NoError(t, app.Boot())

	return genhttp.NewKernel(app, genhttp.KernelConfig{DisableStartupMessage: true})
}

// TestRateLimiterConcurrentAccess drives the limiter from many goroutines at
// once.
//
// The map backing the limiter is shared by every in-flight request. An
// unsynchronised version fails here as "concurrent map read and map write" — a
// runtime fatal error that kills the process and cannot be recovered — so this
// test guards against a remotely triggerable crash, not just a lost counter.
func TestRateLimiterConcurrentAccess(t *testing.T) {
	k := newKernel(t)
	// A limit high enough that nothing is rejected: this test is about safe
	// concurrent access, not about the limit itself.
	k.Use(middleware.RateLimiter(1_000_000, time.Minute))
	k.GET("/", func(ctx *genhttp.Context) error { return ctx.String("ok") })

	var wg sync.WaitGroup
	for range 300 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := k.Fiber().Test(httptest.NewRequest("GET", "/", nil), -1)
			assert.NoError(t, err)
			if resp != nil {
				assert.Equal(t, 200, resp.StatusCode)
			}
		}()
	}
	wg.Wait()
}

// TestRateLimiterEnforcesLimit checks the limiter actually rejects.
func TestRateLimiterEnforcesLimit(t *testing.T) {
	k := newKernel(t)
	k.Use(middleware.RateLimiter(3, time.Minute))
	k.GET("/", func(ctx *genhttp.Context) error { return ctx.String("ok") })

	var statuses []int
	for range 5 {
		resp, err := k.Fiber().Test(httptest.NewRequest("GET", "/", nil), -1)
		require.NoError(t, err)
		statuses = append(statuses, resp.StatusCode)
	}

	assert.Equal(t, []int{200, 200, 200, 429, 429}, statuses)
}

// TestRateLimiterRetryAfterIsSeconds verifies the Retry-After header is a
// delta-seconds value. A Go duration string such as "1m0s" is not valid there
// and clients ignore it.
func TestRateLimiterRetryAfterIsSeconds(t *testing.T) {
	k := newKernel(t)
	k.Use(middleware.RateLimiter(1, 90*time.Second))
	k.GET("/", func(ctx *genhttp.Context) error { return ctx.String("ok") })

	_, err := k.Fiber().Test(httptest.NewRequest("GET", "/", nil), -1)
	require.NoError(t, err)

	resp, err := k.Fiber().Test(httptest.NewRequest("GET", "/", nil), -1)
	require.NoError(t, err)
	require.Equal(t, 429, resp.StatusCode)

	retryAfter := resp.Header.Get("Retry-After")
	seconds, err := strconv.Atoi(retryAfter)
	require.NoErrorf(t, err, "Retry-After %q must be an integer number of seconds", retryAfter)
	assert.Equal(t, 90, seconds)
}

// TestBasicAuthRejectsInvalidCredentials is the regression test for an auth
// bypass: an earlier implementation accepted any non-empty Authorization
// header without ever checking the credentials.
func TestBasicAuthRejectsInvalidCredentials(t *testing.T) {
	k := newKernel(t)
	k.Use(middleware.BasicAuth(map[string]string{"alice": "correct-horse"}))
	k.GET("/secret", func(ctx *genhttp.Context) error { return ctx.String("classified") })

	basic := func(raw string) string {
		return "Basic " + base64.StdEncoding.EncodeToString([]byte(raw))
	}

	cases := []struct {
		name   string
		header string
		want   int
	}{
		{"no header", "", 401},
		{"garbage header", "x", 401},
		{"bearer instead of basic", "Bearer sometoken", 401},
		{"malformed base64", "Basic !!!!not-base64!!!!", 401},
		{"no colon separator", basic("alicecorrect-horse"), 401},
		{"unknown user", basic("mallory:correct-horse"), 401},
		{"known user wrong password", basic("alice:wrong"), 401},
		{"empty password", basic("alice:"), 401},
		{"correct credentials", basic("alice:correct-horse"), 200},
		{"lowercase scheme still valid", "basic " + base64.StdEncoding.EncodeToString([]byte("alice:correct-horse")), 200},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/secret", nil)
			if tc.header != "" {
				req.Header.Set("Authorization", tc.header)
			}

			resp, err := k.Fiber().Test(req, -1)
			require.NoError(t, err)
			assert.Equal(t, tc.want, resp.StatusCode)

			if tc.want == 401 {
				assert.Contains(t, resp.Header.Get("WWW-Authenticate"), "Basic",
					"a rejection must carry a challenge")

				body, _ := io.ReadAll(resp.Body)
				assert.NotContains(t, string(body), "classified",
					"handler body must never leak on a rejected request")
			}
		})
	}
}

// TestTimeoutDoesNotOutliveTheRequest verifies the handler has finished by the
// time the middleware returns.
//
// An implementation that runs the handler in a goroutine and abandons it at
// the deadline leaves that goroutine writing into a *fiber.Ctx which Fiber has
// already recycled onto another connection, corrupting an unrelated client's
// response. Under -race that shows up as a race between Ctx.SendString and
// ReleaseCtx.
func TestTimeoutDoesNotOutliveTheRequest(t *testing.T) {
	k := newKernel(t)
	k.Use(middleware.Timeout(50 * time.Millisecond))

	var (
		mu       sync.Mutex
		finished bool
	)

	k.GET("/slow", func(ctx *genhttp.Context) error {
		// A cooperative handler: it observes the deadline it was given.
		select {
		case <-ctx.Request().Context().Done():
		case <-time.After(5 * time.Second):
		}
		mu.Lock()
		finished = true
		mu.Unlock()
		return ctx.Request().Context().Err()
	})

	resp, err := k.Fiber().Test(httptest.NewRequest("GET", "/slow", nil), -1)
	require.NoError(t, err)
	assert.Equal(t, 408, resp.StatusCode)

	// The handler must be done before the response was produced; no goroutine
	// may still be holding the context.
	mu.Lock()
	defer mu.Unlock()
	assert.True(t, finished, "handler must complete before the middleware returns")
}

// TestTimeoutPassesThroughFastHandlers checks the happy path is untouched.
func TestTimeoutPassesThroughFastHandlers(t *testing.T) {
	k := newKernel(t)
	k.Use(middleware.Timeout(2 * time.Second))
	k.GET("/", func(ctx *genhttp.Context) error { return ctx.String("quick") })

	resp, err := k.Fiber().Test(httptest.NewRequest("GET", "/", nil), -1)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	assert.Equal(t, "quick", string(body))
}

// TestSecureHSTSHeaderIsWellFormed guards the header value formatting.
//
// "max-age=" + string(rune(n)) yields the Unicode character with code point n,
// producing a header browsers silently discard — HSTS would look configured
// while never applying.
func TestSecureHSTSHeaderIsWellFormed(t *testing.T) {
	k := newKernel(t)
	k.Use(middleware.Secure(middleware.SecureConfig{
		HSTSMaxAge:            31536000,
		HSTSIncludeSubdomains: true,
		ContentTypeNosniff:    "nosniff",
	}))
	k.GET("/", func(ctx *genhttp.Context) error { return ctx.String("ok") })

	resp, err := k.Fiber().Test(httptest.NewRequest("GET", "/", nil), -1)
	require.NoError(t, err)

	hsts := resp.Header.Get("Strict-Transport-Security")
	assert.Equal(t, "max-age=31536000; includeSubDomains", hsts)
	assert.True(t, strings.HasPrefix(hsts, "max-age=31536000"))
	assert.Equal(t, "nosniff", resp.Header.Get("X-Content-Type-Options"))
}

// TestSecureOmitsHSTSByDefault: enabling HSTS on a host not yet fully served
// over TLS locks clients out for the max-age lifetime, so it must be opt-in.
func TestSecureOmitsHSTSByDefault(t *testing.T) {
	k := newKernel(t)
	k.Use(middleware.Secure())
	k.GET("/", func(ctx *genhttp.Context) error { return ctx.String("ok") })

	resp, err := k.Fiber().Test(httptest.NewRequest("GET", "/", nil), -1)
	require.NoError(t, err)
	assert.Empty(t, resp.Header.Get("Strict-Transport-Security"))
}

// TestCORSSetsVaryOnOrigin: without Vary, a shared cache can serve one
// origin's Access-Control-Allow-Origin to a different origin.
func TestCORSSetsVaryOnOrigin(t *testing.T) {
	k := newKernel(t)
	k.Use(middleware.CORS(middleware.CORSConfig{
		AllowOrigins: "https://app.example.com,https://admin.example.com",
		AllowMethods: "GET",
		MaxAge:       3600,
	}))
	k.GET("/", func(ctx *genhttp.Context) error { return ctx.String("ok") })

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Origin", "https://app.example.com")
	resp, err := k.Fiber().Test(req, -1)
	require.NoError(t, err)

	assert.Equal(t, "https://app.example.com", resp.Header.Get("Access-Control-Allow-Origin"))
	assert.Contains(t, resp.Header.Get("Vary"), "Origin")
	// Max-Age must be a number, not a Unicode code point.
	assert.Equal(t, "3600", resp.Header.Get("Access-Control-Max-Age"))
}

// TestCORSRejectsUnlistedOrigin verifies a non-allowlisted origin gets no CORS
// grant.
func TestCORSRejectsUnlistedOrigin(t *testing.T) {
	k := newKernel(t)
	k.Use(middleware.CORS(middleware.CORSConfig{AllowOrigins: "https://app.example.com"}))
	k.GET("/", func(ctx *genhttp.Context) error { return ctx.String("ok") })

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Origin", "https://evil.example")
	resp, err := k.Fiber().Test(req, -1)
	require.NoError(t, err)

	assert.Empty(t, resp.Header.Get("Access-Control-Allow-Origin"))
}

// TestCORSNeverPairsWildcardWithCredentials: browsers reject "*" alongside
// Allow-Credentials, so the middleware must echo the concrete origin instead.
func TestCORSNeverPairsWildcardWithCredentials(t *testing.T) {
	k := newKernel(t)
	k.Use(middleware.CORS(middleware.CORSConfig{
		AllowOrigins:     "*",
		AllowCredentials: true,
	}))
	k.GET("/", func(ctx *genhttp.Context) error { return ctx.String("ok") })

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Origin", "https://app.example.com")
	resp, err := k.Fiber().Test(req, -1)
	require.NoError(t, err)

	assert.Equal(t, "true", resp.Header.Get("Access-Control-Allow-Credentials"))
	assert.NotEqual(t, "*", resp.Header.Get("Access-Control-Allow-Origin"))
	assert.Equal(t, "https://app.example.com", resp.Header.Get("Access-Control-Allow-Origin"))
	// The origin is echoed from the request even though "*" is configured, so
	// the response is origin-dependent and must not be cached across origins.
	assert.Contains(t, resp.Header.Get("Vary"), "Origin")
}
