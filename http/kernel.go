package http

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/genesysflow/go-genesys/container"
	"github.com/genesysflow/go-genesys/contracts"
	"github.com/gofiber/fiber/v2"
)

// Kernel is the HTTP kernel that handles the request lifecycle.
type Kernel struct {
	app        contracts.Application
	fiber      *fiber.App
	router     *Router
	middleware []MiddlewareFunc
	logger     contracts.Logger
}

// KernelConfig defines configuration for the HTTP kernel.
type KernelConfig struct {
	// Fiber configuration
	AppName               string
	Prefork               bool
	ServerHeader          string
	StrictRouting         bool
	CaseSensitive         bool
	BodyLimit             int
	ReadTimeout           time.Duration
	WriteTimeout          time.Duration
	IdleTimeout           time.Duration
	ReadBufferSize        int
	WriteBufferSize       int
	EnablePrintRoutes     bool
	DisableStartupMessage bool

	// Genesys-specific
	TrustedProxies []string
}

// DefaultKernelConfig returns the default kernel configuration.
func DefaultKernelConfig() KernelConfig {
	return KernelConfig{
		AppName:           "Genesys",
		ServerHeader:      "Genesys",
		BodyLimit:         4 * 1024 * 1024, // 4MB
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
		ReadBufferSize:    4096,
		WriteBufferSize:   4096,
		EnablePrintRoutes: false,
	}
}

// NewKernel creates a new HTTP kernel.
func NewKernel(app contracts.Application, config ...KernelConfig) *Kernel {
	cfg := DefaultKernelConfig()
	if len(config) > 0 {
		cfg = config[0]
	}

	// Create Fiber app with configuration
	fiberApp := fiber.New(fiber.Config{
		AppName:               cfg.AppName,
		Prefork:               cfg.Prefork,
		ServerHeader:          cfg.ServerHeader,
		StrictRouting:         cfg.StrictRouting,
		CaseSensitive:         cfg.CaseSensitive,
		BodyLimit:             cfg.BodyLimit,
		ReadTimeout:           cfg.ReadTimeout,
		WriteTimeout:          cfg.WriteTimeout,
		IdleTimeout:           cfg.IdleTimeout,
		ReadBufferSize:        cfg.ReadBufferSize,
		WriteBufferSize:       cfg.WriteBufferSize,
		EnablePrintRoutes:     cfg.EnablePrintRoutes,
		DisableStartupMessage: cfg.DisableStartupMessage,
		ErrorHandler:          createErrorHandler(app),
	})

	// Note: Trusted proxies are set via fiber.Config during app creation
	// For Fiber v2, EnableTrustedProxyCheck and TrustedProxies should be
	// passed in the fiber.Config struct when creating the app

	// Get logger from container
	logger := container.MustResolve[contracts.Logger](app)

	kernel := &Kernel{
		app:        app,
		fiber:      fiberApp,
		middleware: make([]MiddlewareFunc, 0),
		logger:     logger,
	}

	// Create router
	kernel.router = NewRouter(app, fiberApp)

	return kernel
}

// createErrorHandler creates the Fiber error handler.
func createErrorHandler(app contracts.Application) fiber.ErrorHandler {
	return func(c *fiber.Ctx, err error) error {
		// Try to resolve the error handler
		// We use a local interface to avoid import cycle with errors package
		type ErrorHandler interface {
			Handle(ctx contracts.Context, err error) error
		}

		if h, resolveErr := container.Resolve[any](app, "error.handler"); resolveErr == nil {
			if handler, ok := h.(ErrorHandler); ok {
				// Create context
				ctx := NewContext(c, app)
				return handler.Handle(ctx, err)
			}
		}

		code := fiber.StatusInternalServerError

		// Check if it's a Fiber error
		if e, ok := err.(*fiber.Error); ok {
			code = e.Code
		}

		// Check if it's an HTTP error from contracts
		if e, ok := err.(contracts.HTTPError); ok {
			code = e.StatusCode()
		}

		// Log the error (use different variable name to avoid shadowing)
		if logger, resolveErr := container.Resolve[contracts.Logger](app); resolveErr == nil {
			logger.Error("HTTP Error",
				"error", err.Error(),
				"status", code,
				"path", c.Path(),
				"method", c.Method(),
			)
		}

		// Return JSON error for API requests
		if c.Accepts("application/json") == "application/json" {
			return c.Status(code).JSON(fiber.Map{
				"error":   err.Error(),
				"status":  code,
				"success": false,
			})
		}

		// Return plain text for other requests
		return c.Status(code).SendString(err.Error())
	}
}

// Fiber returns the underlying Fiber app.
func (k *Kernel) Fiber() *fiber.App {
	return k.fiber
}

// Router returns the router instance.
func (k *Kernel) Router() *Router {
	return k.router
}

// Use registers global middleware.
func (k *Kernel) Use(middleware ...MiddlewareFunc) *Kernel {
	k.middleware = append(k.middleware, middleware...)
	k.router.Use(middleware...)
	return k
}

// UseFiber registers Fiber middleware directly.
func (k *Kernel) UseFiber(middleware ...fiber.Handler) *Kernel {
	for _, m := range middleware {
		k.fiber.Use(m)
	}
	return k
}

// Run starts the HTTP server.
func (k *Kernel) Run(addr string) error {
	if k.logger != nil {
		k.logger.Info("Starting HTTP server", "address", addr)
	}

	return k.fiber.Listen(addr)
}

// RunTLS starts the HTTP server with TLS.
func (k *Kernel) RunTLS(addr, certFile, keyFile string) error {
	if k.logger != nil {
		k.logger.Info("Starting HTTPS server", "address", addr)
	}

	return k.fiber.ListenTLS(addr, certFile, keyFile)
}

// RunWithGracefulShutdown starts the server with graceful shutdown support.
func (k *Kernel) RunWithGracefulShutdown(addr string, timeout time.Duration) error {
	// Channel to listen for errors from the server
	errChan := make(chan error, 1)

	// Start server in goroutine
	go func() {
		if k.logger != nil {
			k.logger.Info("Starting HTTP server with graceful shutdown", "address", addr)
		}
		errChan <- k.fiber.Listen(addr)
	}()

	// Channel to listen for interrupt signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Wait for interrupt or error
	select {
	case err := <-errChan:
		return err
	case sig := <-sigChan:
		if k.logger != nil {
			k.logger.Info("Received shutdown signal", "signal", sig.String())
		}
	}

	// Create shutdown context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Terminate the application
	// Note: We ignore the error here as samber/do may return non-critical marshaling errors
	_ = k.app.TerminateWithContext(ctx)

	// Shutdown Fiber
	if err := k.fiber.ShutdownWithContext(ctx); err != nil {
		return fmt.Errorf("server shutdown failed: %w", err)
	}

	if k.logger != nil {
		k.logger.Info("Server shutdown completed")
	}

	return nil
}

// Shutdown gracefully shuts down the server.
func (k *Kernel) Shutdown() error {
	return k.fiber.Shutdown()
}

// ShutdownWithContext gracefully shuts down the server with context.
func (k *Kernel) ShutdownWithContext(ctx context.Context) error {
	return k.fiber.ShutdownWithContext(ctx)
}

// Test returns a test client for the application.
// Useful for testing HTTP handlers.
func (k *Kernel) Test(req *TestRequest) (*TestResponse, error) {
	resp, err := k.fiber.Test(req.toHTTPRequest(), -1)
	if err != nil {
		return nil, err
	}
	return newTestResponse(resp), nil
}

// Static serves static files.
func (k *Kernel) Static(prefix, root string) {
	k.router.Static(prefix, root)
}

// GET registers a GET route.
func (k *Kernel) GET(path string, handler HandlerFunc, middleware ...MiddlewareFunc) *Route {
	return k.router.GET(path, handler, middleware...)
}

// POST registers a POST route.
func (k *Kernel) POST(path string, handler HandlerFunc, middleware ...MiddlewareFunc) *Route {
	return k.router.POST(path, handler, middleware...)
}

// PUT registers a PUT route.
func (k *Kernel) PUT(path string, handler HandlerFunc, middleware ...MiddlewareFunc) *Route {
	return k.router.PUT(path, handler, middleware...)
}

// PATCH registers a PATCH route.
func (k *Kernel) PATCH(path string, handler HandlerFunc, middleware ...MiddlewareFunc) *Route {
	return k.router.PATCH(path, handler, middleware...)
}

// DELETE registers a DELETE route.
func (k *Kernel) DELETE(path string, handler HandlerFunc, middleware ...MiddlewareFunc) *Route {
	return k.router.DELETE(path, handler, middleware...)
}

// Group creates a route group.
func (k *Kernel) Group(prefix string, fn func(*Router), middleware ...MiddlewareFunc) {
	k.router.Group(prefix, fn, middleware...)
}
