package commands

import (
	"testing"
	"time"

	"github.com/genesysflow/go-genesys/container"
	"github.com/genesysflow/go-genesys/foundation"
	"github.com/genesysflow/go-genesys/http"
	"github.com/genesysflow/go-genesys/providers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServeCommand_WithoutKernelConfig(t *testing.T) {
	// Create a test application
	app := foundation.New()

	// Register logger provider
	app.Register(&providers.LogServiceProvider{})

	// Define test routes
	testRoutes := func(r *http.Router) {
		r.GET("/test", func(ctx *http.Context) error {
			return ctx.JSONResponse(map[string]any{"test": "ok"})
		})
	}
	app.InstanceType(testRoutes)

	// Boot the app
	err := app.Boot()
	require.NoError(t, err)

	// Resolve routes to verify it's registered
	routes, err := container.Resolve[func(*http.Router)](app)
	assert.NoError(t, err)
	assert.NotNil(t, routes)

	// Verify kernel config is not in container
	_, err = container.Resolve[*http.KernelConfig](app)
	assert.Error(t, err, "kernel config should not be in container")
}

func TestServeCommand_WithKernelConfig(t *testing.T) {
	// Create a test application
	app := foundation.New()

	// Register logger provider
	app.Register(&providers.LogServiceProvider{})

	// Define test routes
	testRoutes := func(r *http.Router) {
		r.GET("/test", func(ctx *http.Context) error {
			return ctx.JSONResponse(map[string]any{"test": "ok"})
		})
	}
	app.InstanceType(testRoutes)

	// Define custom kernel config
	kernelConfig := &http.KernelConfig{
		AppName:           "TestApp",
		ServerHeader:      "TestServer",
		BodyLimit:         100 * 1024 * 1024, // 100MB
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      60 * time.Second,
		EnablePrintRoutes: true,
	}
	app.InstanceType(kernelConfig)

	// Boot the app
	err := app.Boot()
	require.NoError(t, err)

	// Resolve kernel config to verify it's registered
	resolvedConfig, err := container.Resolve[*http.KernelConfig](app)
	assert.NoError(t, err)
	assert.NotNil(t, resolvedConfig)
	assert.Equal(t, "TestApp", resolvedConfig.AppName)
	assert.Equal(t, "TestServer", resolvedConfig.ServerHeader)
	assert.Equal(t, 100*1024*1024, resolvedConfig.BodyLimit)
	assert.Equal(t, 60*time.Second, resolvedConfig.ReadTimeout)
	assert.Equal(t, 60*time.Second, resolvedConfig.WriteTimeout)
	assert.Equal(t, true, resolvedConfig.EnablePrintRoutes)
}

func TestServeCommand_KernelConfigAppliedToProvider(t *testing.T) {
	// Create a test application
	app := foundation.New()

	// Register logger provider
	app.Register(&providers.LogServiceProvider{})

	// Define test routes
	testRoutes := func(r *http.Router) {
		r.GET("/test", func(ctx *http.Context) error {
			return ctx.JSONResponse(map[string]any{"test": "ok"})
		})
	}

	// Define custom kernel config with specific body limit
	expectedBodyLimit := 100 * 1024 * 1024 // 100MB
	kernelConfig := &http.KernelConfig{
		AppName:      "TestApp",
		ServerHeader: "TestServer",
		BodyLimit:    expectedBodyLimit,
	}

	// Simulate what runServer does
	var routesCallback func(*http.Router)
	var kernelConfigFromContainer *http.KernelConfig
	var globalMiddleware []http.MiddlewareFunc

	app.InstanceType(testRoutes)
	app.InstanceType(kernelConfig)

	// Simulate resolution from container
	if routes, err := container.Resolve[func(*http.Router)](app); err == nil {
		routesCallback = routes
	}

	if cfg, err := container.Resolve[*http.KernelConfig](app); err == nil {
		kernelConfigFromContainer = cfg
	}

	if mw, err := container.Resolve[[]http.MiddlewareFunc](app); err == nil {
		globalMiddleware = mw
	}

	// Create route provider with resolved config
	routeProvider := &providers.RouteServiceProvider{
		Routes:       routesCallback,
		Middleware:   globalMiddleware,
		KernelConfig: kernelConfigFromContainer,
	}

	// Verify config was resolved and passed to provider
	assert.NotNil(t, routeProvider.KernelConfig, "kernel config should be set on provider")
	assert.Equal(t, expectedBodyLimit, routeProvider.KernelConfig.BodyLimit, "body limit should match")
	assert.Equal(t, "TestApp", routeProvider.KernelConfig.AppName)
	assert.Equal(t, "TestServer", routeProvider.KernelConfig.ServerHeader)

	// Register and boot to ensure it works end-to-end
	app.Register(routeProvider)
	err := app.Boot()
	require.NoError(t, err)

	// Verify the kernel was created with the config
	kernel := routeProvider.Kernel()
	assert.NotNil(t, kernel)
}

func TestServeCommand_DefaultsWhenNoConfig(t *testing.T) {
	// Create a test application
	app := foundation.New()

	// Register logger provider
	app.Register(&providers.LogServiceProvider{})

	// Simulate what runServer does without any config
	var routesCallback func(*http.Router)
	var kernelConfigFromContainer *http.KernelConfig
	var globalMiddleware []http.MiddlewareFunc

	// Try to resolve (should fail gracefully)
	if routes, err := container.Resolve[func(*http.Router)](app); err == nil {
		routesCallback = routes
	}

	if cfg, err := container.Resolve[*http.KernelConfig](app); err == nil {
		kernelConfigFromContainer = cfg
	}

	if mw, err := container.Resolve[[]http.MiddlewareFunc](app); err == nil {
		globalMiddleware = mw
	}

	// Create route provider without config (should use defaults)
	routeProvider := &providers.RouteServiceProvider{
		Routes:       routesCallback,
		Middleware:   globalMiddleware,
		KernelConfig: kernelConfigFromContainer,
	}

	// Verify config is nil (will use defaults in the provider)
	assert.Nil(t, routeProvider.KernelConfig, "kernel config should be nil when not provided")

	// Register and boot to ensure it works with defaults
	app.Register(routeProvider)
	err := app.Boot()
	require.NoError(t, err)

	// Verify the kernel was created with default config
	kernel := routeProvider.Kernel()
	assert.NotNil(t, kernel)
}

func TestServeCommand_Creation(t *testing.T) {
	// Create a test application
	app := foundation.New()

	// Register logger provider
	app.Register(&providers.LogServiceProvider{})
	err := app.Boot()
	require.NoError(t, err)

	// Create the serve command
	cmd := ServeCommand(app)

	assert.NotNil(t, cmd)
	assert.Equal(t, "serve", cmd.Use)
	assert.Equal(t, "Run the development server", cmd.Short)

	// Verify flags
	portFlag := cmd.Flags().Lookup("port")
	assert.NotNil(t, portFlag)
	assert.Equal(t, "3000", portFlag.DefValue)

	hostFlag := cmd.Flags().Lookup("host")
	assert.NotNil(t, hostFlag)
	assert.Equal(t, "localhost", hostFlag.DefValue)
}

func TestServeCommand_MultipleConfigs(t *testing.T) {
	// Test that last config wins if multiple are registered
	app := foundation.New()

	// Register logger provider
	app.Register(&providers.LogServiceProvider{})

	// Register first config
	firstConfig := &http.KernelConfig{
		AppName:   "FirstApp",
		BodyLimit: 10 * 1024 * 1024,
	}
	app.InstanceType(firstConfig)

	// This should overwrite the first one
	secondConfig := &http.KernelConfig{
		AppName:   "SecondApp",
		BodyLimit: 50 * 1024 * 1024,
	}
	app.InstanceType(secondConfig)

	// Resolve should get the latest one
	resolvedConfig, err := container.Resolve[*http.KernelConfig](app)
	assert.NoError(t, err)

	// Note: Depending on container behavior, this might be the last registered
	// In Go-Genesys, InstanceType should bind as singleton, so last one wins
	assert.Equal(t, "SecondApp", resolvedConfig.AppName)
	assert.Equal(t, 50*1024*1024, resolvedConfig.BodyLimit)
}
