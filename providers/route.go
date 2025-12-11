package providers

import (
	"reflect"

	"github.com/genesysflow/go-genesys/container"
	"github.com/genesysflow/go-genesys/contracts"
	"github.com/genesysflow/go-genesys/http"
)

// RouteServiceProvider registers HTTP routing services.
type RouteServiceProvider struct {
	BaseProvider

	// Routes is a function that defines the application routes.
	// Set this before registering the provider.
	Routes func(router *http.Router)

	// Middleware is a list of global middleware to apply.
	Middleware []http.MiddlewareFunc

	// KernelConfig is optional kernel configuration.
	KernelConfig *http.KernelConfig

	// kernel is the cached HTTP kernel
	kernel *http.Kernel
}

// Register registers the routing services.
func (p *RouteServiceProvider) Register(app contracts.Application) error {
	p.app = app

	// Create HTTP kernel
	if p.KernelConfig != nil {
		p.kernel = http.NewKernel(app, *p.KernelConfig)
	} else {
		p.kernel = http.NewKernel(app)
	}

	// Apply global middleware
	if len(p.Middleware) > 0 {
		p.kernel.Use(p.Middleware...)
	}

	// Bind to container with type-safe registration
	// Access the embedded Container field via reflection to avoid import cycles
	appValue := reflect.ValueOf(app)
	if appValue.Kind() == reflect.Ptr {
		appValue = appValue.Elem()
	}
	
	containerField := appValue.FieldByName("Container")
	if containerField.IsValid() && !containerField.IsNil() {
		// Extract the *container.Container from the reflect.Value
		containerPtr := containerField.Interface().(*container.Container)
		// Use type-safe ProvideNamedValue
		container.ProvideNamedValue[*http.Kernel](containerPtr, "http.kernel", p.kernel)
		container.ProvideNamedValue[*http.Router](containerPtr, "router", p.kernel.Router())
	} else {
		// Fallback to BindValue if Container field not accessible
		app.BindValue("http.kernel", p.kernel)
		app.BindValue("router", p.kernel.Router())
	}

	return nil
}

// Boot bootstraps the routing services.
func (p *RouteServiceProvider) Boot(app contracts.Application) error {
	// Register routes if defined
	if p.Routes != nil && p.kernel != nil {
		p.Routes(p.kernel.Router())
	}

	return nil
}

// Provides returns the services this provider registers.
func (p *RouteServiceProvider) Provides() []string {
	return []string{
		"http.kernel",
		"router",
	}
}

// Kernel returns the HTTP kernel.
func (p *RouteServiceProvider) Kernel() *http.Kernel {
	return p.kernel
}
