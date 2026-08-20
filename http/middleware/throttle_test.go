package middleware_test

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/genesysflow/go-genesys/cache"
	"github.com/genesysflow/go-genesys/foundation"
	genhttp "github.com/genesysflow/go-genesys/http"
	"github.com/genesysflow/go-genesys/http/middleware"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func throttledKernel(t *testing.T, config middleware.ThrottleConfig) *genhttp.Kernel {
	t.Helper()
	app := foundation.New()
	require.NoError(t, app.Boot())
	kernel := genhttp.NewKernel(app, genhttp.KernelConfig{DisableStartupMessage: true})
	kernel.GET("/limited", func(ctx *genhttp.Context) error {
		return ctx.String("ok")
	}, middleware.Throttle(config))
	return kernel
}

func TestThrottleLimitsAndHeaders(t *testing.T) {
	kernel := throttledKernel(t, middleware.ThrottleConfig{
		Store:       cache.NewMemoryStore(),
		Name:        "api",
		MaxRequests: 3,
		Window:      time.Minute,
	})

	for i := 1; i <= 3; i++ {
		resp, err := kernel.Fiber().Test(httptest.NewRequest("GET", "/limited", nil), -1)
		require.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode, "request %d within the limit", i)
		assert.Equal(t, "3", resp.Header.Get("X-RateLimit-Limit"))
	}

	resp, err := kernel.Fiber().Test(httptest.NewRequest("GET", "/limited", nil), -1)
	require.NoError(t, err)
	assert.Equal(t, 429, resp.StatusCode)
	assert.Equal(t, "0", resp.Header.Get("X-RateLimit-Remaining"))
	assert.Equal(t, "60", resp.Header.Get("Retry-After"))
}

func TestThrottleWindowResets(t *testing.T) {
	kernel := throttledKernel(t, middleware.ThrottleConfig{
		Store:       cache.NewMemoryStore(),
		MaxRequests: 1,
		Window:      50 * time.Millisecond,
	})

	resp, err := kernel.Fiber().Test(httptest.NewRequest("GET", "/limited", nil), -1)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	resp, err = kernel.Fiber().Test(httptest.NewRequest("GET", "/limited", nil), -1)
	require.NoError(t, err)
	assert.Equal(t, 429, resp.StatusCode)

	time.Sleep(80 * time.Millisecond)
	resp, err = kernel.Fiber().Test(httptest.NewRequest("GET", "/limited", nil), -1)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode, "allowance returns after the window")
}

func TestThrottleSeparatesKeys(t *testing.T) {
	kernel := throttledKernel(t, middleware.ThrottleConfig{
		Store:       cache.NewMemoryStore(),
		MaxRequests: 1,
		Window:      time.Minute,
		KeyFor: func(ctx *genhttp.Context) string {
			return ctx.Request().Header("X-User")
		},
	})

	send := func(user string) int {
		req := httptest.NewRequest("GET", "/limited", nil)
		req.Header.Set("X-User", user)
		resp, err := kernel.Fiber().Test(req, -1)
		require.NoError(t, err)
		return resp.StatusCode
	}

	assert.Equal(t, 200, send("alice"))
	assert.Equal(t, 429, send("alice"), "alice used her allowance")
	assert.Equal(t, 200, send("bob"), "bob has his own allowance")
}
