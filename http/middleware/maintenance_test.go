package middleware_test

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/genesysflow/go-genesys/foundation"
	genhttp "github.com/genesysflow/go-genesys/http"
	"github.com/genesysflow/go-genesys/http/middleware"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func maintenanceKernel(t *testing.T, downPath string) *genhttp.Kernel {
	t.Helper()
	app := foundation.New()
	require.NoError(t, app.Boot())
	kernel := genhttp.NewKernel(app, genhttp.KernelConfig{DisableStartupMessage: true})
	kernel.Use(middleware.Maintenance(middleware.MaintenanceConfig{Path: downPath}))
	kernel.GET("/", func(ctx *genhttp.Context) error { return ctx.String("live") })
	return kernel
}

func TestMaintenanceMode(t *testing.T) {
	downPath := filepath.Join(t.TempDir(), "down")
	kernel := maintenanceKernel(t, downPath)

	// No down file: traffic flows.
	resp, err := kernel.Fiber().Test(httptest.NewRequest("GET", "/", nil), -1)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// Down file present: 503 with Retry-After and the custom message.
	require.NoError(t, os.WriteFile(downPath,
		[]byte(`{"message":"Back soon","retry_after":120,"secret":"letmein"}`), 0o644))

	resp, err = kernel.Fiber().Test(httptest.NewRequest("GET", "/", nil), -1)
	require.NoError(t, err)
	assert.Equal(t, 503, resp.StatusCode)
	assert.Equal(t, "120", resp.Header.Get("Retry-After"))
	body := make([]byte, 128)
	n, _ := resp.Body.Read(body)
	assert.Contains(t, string(body[:n]), "Back soon")

	// The secret bypasses and plants a cookie.
	resp, err = kernel.Fiber().Test(httptest.NewRequest("GET", "/?secret=letmein", nil), -1)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	var bypassCookie string
	for _, c := range resp.Cookies() {
		if c.Name == "genesys_maintenance" {
			bypassCookie = c.Value
		}
	}
	require.NotEmpty(t, bypassCookie)

	// The cookie alone keeps working; a wrong secret does not.
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Cookie", "genesys_maintenance="+bypassCookie)
	resp, err = kernel.Fiber().Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	resp, err = kernel.Fiber().Test(httptest.NewRequest("GET", "/?secret=wrong", nil), -1)
	require.NoError(t, err)
	assert.Equal(t, 503, resp.StatusCode)

	// Removing the file restores traffic instantly.
	require.NoError(t, os.Remove(downPath))
	resp, err = kernel.Fiber().Test(httptest.NewRequest("GET", "/", nil), -1)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}
