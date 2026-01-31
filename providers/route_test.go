package providers

import (
	"testing"

	"github.com/genesysflow/go-genesys/http"
	"github.com/stretchr/testify/assert"
)

// Note: RouteServiceProvider tests require a full Application with proper
// container setup because NewKernel uses container.MustResolve for the logger.
// These are tested as integration tests with the real Application.

func TestRouteServiceProviderProvides(t *testing.T) {
	provider := &RouteServiceProvider{}
	provides := provider.Provides()

	assert.Contains(t, provides, "http.kernel")
	assert.Contains(t, provides, "router")
}

func TestRouteServiceProviderKernelMethod(t *testing.T) {
	provider := &RouteServiceProvider{}

	// Before register, kernel should be nil
	assert.Nil(t, provider.Kernel())
}

func TestRouteServiceProviderWithKernelConfig(t *testing.T) {
	// Test that config struct can be created
	provider := &RouteServiceProvider{
		KernelConfig: &http.KernelConfig{
			ReadTimeout:  30,
			WriteTimeout: 30,
			IdleTimeout:  60,
		},
	}

	assert.NotNil(t, provider.KernelConfig)
	assert.Equal(t, 30, int(provider.KernelConfig.ReadTimeout))
}

func TestRouteServiceProviderWithMiddleware(t *testing.T) {
	testMiddleware := func(ctx *http.Context, next func() error) error {
		ctx.Set("middleware-applied", true)
		return next()
	}

	provider := &RouteServiceProvider{
		Middleware: []http.MiddlewareFunc{testMiddleware},
	}

	assert.Len(t, provider.Middleware, 1)
}

func TestRouteServiceProviderWithRoutes(t *testing.T) {
	routesCalled := false
	provider := &RouteServiceProvider{
		Routes: func(router *http.Router) {
			routesCalled = true
		},
	}

	// Routes function is set but not called until Register/Boot
	assert.NotNil(t, provider.Routes)
	assert.False(t, routesCalled)
}
