package commands

import (
	"fmt"

	"github.com/genesysflow/go-genesys/container"
	"github.com/genesysflow/go-genesys/contracts"
	"github.com/genesysflow/go-genesys/http"
	"github.com/genesysflow/go-genesys/http/middleware"
	"github.com/genesysflow/go-genesys/providers"
	"github.com/spf13/cobra"
)

// ServeCommand creates the serve command.
func ServeCommand(app contracts.Application) *cobra.Command {
	var port string
	var host string

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Run the development server",
		Long:  `Start the development server for your application.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runServer(app, host, port)
		},
	}

	cmd.Flags().StringVarP(&port, "port", "p", "3000", "Port to run the server on")
	cmd.Flags().StringVarP(&host, "host", "H", "localhost", "Host to bind the server to")

	return cmd
}

func runServer(app contracts.Application, host, port string) error {
	logger := app.GetLogger()

	// Try to get routes callback from container
	var routesCallback func(*http.Router)
	if routes, err := container.Resolve[func(*http.Router)](app); err == nil {
		routesCallback = routes
	}

	// Try to get middleware from container
	var globalMiddleware []http.MiddlewareFunc
	if mw, err := container.Resolve[[]http.MiddlewareFunc](app); err == nil {
		globalMiddleware = mw
	}

	// Try to get kernel config from container (optional).
	// If not found, the RouteServiceProvider will use http.DefaultKernelConfig()
	// with sensible defaults (4MB body limit, 30s timeouts, etc.).
	// Register a custom config via app.InstanceType(&http.KernelConfig{...}) to override.
	var kernelConfig *http.KernelConfig
	if cfg, err := container.Resolve[*http.KernelConfig](app); err == nil {
		kernelConfig = cfg
	}

	// If no routes callback, use default
	if routesCallback == nil {
		routesCallback = func(r *http.Router) {
			r.GET("/", func(ctx *http.Context) error {
				return ctx.JSONResponse(map[string]any{
					"message": "Welcome to Go-Genesys!",
					"version": "1.0.0",
				})
			})

			r.GET("/health", func(ctx *http.Context) error {
				return ctx.JSONResponse(map[string]any{
					"status": "ok",
				})
			})
		}
	}

	// If no middleware, use defaults
	if globalMiddleware == nil {
		globalMiddleware = []http.MiddlewareFunc{
			middleware.RequestID(),
			middleware.Recover(logger),
			middleware.Logger(logger),
			middleware.CORS(),
		}
	}

	// Create route service provider
	routeProvider := &providers.RouteServiceProvider{
		Routes:       routesCallback,
		Middleware:   globalMiddleware,
		KernelConfig: kernelConfig,
	}
	app.Register(routeProvider)

	if err := app.Boot(); err != nil {
		return fmt.Errorf("failed to boot application: %w", err)
	}

	kernel := routeProvider.Kernel()

	logger.Info("Starting server", "host", host, "port", port)
	fmt.Printf("Server starting at http://%s:%s\n", host, port)

	return kernel.RunWithGracefulShutdown(":"+port, 10)
}
