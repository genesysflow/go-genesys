package main

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/genesysflow/go-genesys/console"
	"github.com/genesysflow/go-genesys/container"
	"github.com/genesysflow/go-genesys/example/bootstrap"
	genhttp "github.com/genesysflow/go-genesys/http"
	"github.com/genesysflow/go-genesys/http/middleware"
	"github.com/genesysflow/go-genesys/providers"
)

// TestExampleAppBoots is the wiring smoke test: it boots the real
// example application - every provider, config files, routes - builds
// the HTTP kernel the way `serve` does, and hits live endpoints. A
// provider registration or route wiring regression fails here even
// when every unit test passes.
func TestExampleAppBoots(t *testing.T) {
	app := bootstrap.App()

	routesCallback, err := container.Resolve[func(*genhttp.Router)](app)
	if err != nil {
		t.Fatalf("routes callback not registered: %v", err)
	}
	globalMiddleware, err := container.Resolve[[]genhttp.MiddlewareFunc](app)
	if err != nil {
		t.Fatalf("global middleware not registered: %v", err)
	}
	kernelConfig, err := container.Resolve[*genhttp.KernelConfig](app)
	if err != nil {
		t.Fatalf("kernel config not registered: %v", err)
	}
	kernelConfig.DisableStartupMessage = true

	routeProvider := &providers.RouteServiceProvider{
		Routes:       routesCallback,
		Middleware:   globalMiddleware,
		KernelConfig: kernelConfig,
	}
	if err := app.Register(routeProvider); err != nil {
		t.Fatalf("failed to register route provider: %v", err)
	}
	if err := app.Boot(); err != nil {
		t.Fatalf("application failed to boot: %v", err)
	}
	t.Cleanup(func() { app.Terminate() })

	kernel := routeProvider.Kernel()
	if kernel == nil {
		t.Fatal("route provider produced no HTTP kernel")
	}

	get := func(path string) (int, map[string]any) {
		t.Helper()
		resp, err := kernel.Fiber().Test(httptest.NewRequest("GET", path, nil), -1)
		if err != nil {
			t.Fatalf("request %s failed: %v", path, err)
		}
		content, _ := io.ReadAll(resp.Body)
		var decoded map[string]any
		json.Unmarshal(content, &decoded)
		return resp.StatusCode, decoded
	}

	// Maintenance mode leaves a marker file on disk, so the app has to
	// start from a known-up state. A marker that already exists in the
	// developer's tree is snapshotted and restored when the test ends.
	downPath := filepath.Join(app.BasePath(), middleware.DefaultDownFilePath)
	if existing, err := os.ReadFile(downPath); err == nil {
		t.Cleanup(func() { os.WriteFile(downPath, existing, 0o644) })
		if err := os.Remove(downPath); err != nil {
			t.Fatalf("failed to clear pre-existing down file: %v", err)
		}
	} else {
		t.Cleanup(func() { os.Remove(downPath) })
	}

	// The welcome route responds with the app payload.
	status, body := get("/")
	if status != 200 {
		t.Fatalf("GET / returned %d", status)
	}
	if body["message"] != "Welcome to Go-Genesys Example!" {
		t.Fatalf("unexpected welcome payload: %v", body)
	}

	// The health check answers.
	status, body = get("/health")
	if status != 200 || body["status"] != "healthy" {
		t.Fatalf("health check failed: %d %v", status, body)
	}

	// Unknown routes 404 rather than crash.
	status, _ = get("/definitely-not-a-route")
	if status != 404 {
		t.Fatalf("expected 404 for unknown route, got %d", status)
	}

	// `example down` writes the maintenance marker; the global middleware
	// stack has to turn it into a 503 for every route until `example up`
	// removes it again. Without middleware.Maintenance registered the app
	// keeps serving traffic and the operator never notices.
	cli := container.MustResolve[*console.Kernel](app, "console.kernel")
	if err := cli.Handle([]string{"down", "--message", "Upgrading", "--retry", "120"}); err != nil {
		t.Fatalf("down command failed: %v", err)
	}

	status, body = get("/")
	if status != 503 {
		t.Fatalf("expected 503 while in maintenance mode, got %d", status)
	}
	if body["message"] != "Upgrading" {
		t.Fatalf("unexpected maintenance payload: %v", body)
	}

	// The --retry flag surfaces as Retry-After. The get helper only
	// returns status and decoded body, so inspect the raw response.
	resp, err := kernel.Fiber().Test(httptest.NewRequest("GET", "/", nil), -1)
	if err != nil {
		t.Fatalf("maintenance request failed: %v", err)
	}
	if retryAfter := resp.Header.Get("Retry-After"); retryAfter != "120" {
		t.Fatalf("expected Retry-After 120, got %q", retryAfter)
	}

	// Maintenance mode is global, not just the welcome route.
	status, _ = get("/health")
	if status != 503 {
		t.Fatalf("expected 503 for health check while down, got %d", status)
	}

	if err := cli.Handle([]string{"up"}); err != nil {
		t.Fatalf("up command failed: %v", err)
	}
	status, _ = get("/")
	if status != 200 {
		t.Fatalf("expected 200 after leaving maintenance mode, got %d", status)
	}
}
