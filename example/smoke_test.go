package main

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"testing"

	"github.com/genesysflow/go-genesys/container"
	"github.com/genesysflow/go-genesys/example/bootstrap"
	genhttp "github.com/genesysflow/go-genesys/http"
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
	middleware, err := container.Resolve[[]genhttp.MiddlewareFunc](app)
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
		Middleware:   middleware,
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
}
