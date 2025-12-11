// Package main demonstrates Go-Genesys framework usage.
package main

import (
	"log"
	"os"

	"github.com/genesysflow/go-genesys/example/app/controllers"
	"github.com/genesysflow/go-genesys/foundation"
	"github.com/genesysflow/go-genesys/http"
	"github.com/genesysflow/go-genesys/http/middleware"
	"github.com/genesysflow/go-genesys/providers"
	"github.com/genesysflow/go-genesys/validation"
)

func main() {
	// Create a new application instance with base path
	app := foundation.New(".")

	// Register service providers
	app.Register(&providers.AppServiceProvider{})
	app.Register(&providers.LogServiceProvider{})
	app.Register(&providers.ValidationServiceProvider{})
	app.Register(&providers.SessionServiceProvider{})

	// Create route service provider with routes and middleware
	routeProvider := &providers.RouteServiceProvider{
		Routes: func(router *http.Router) {
			registerRoutes(router, app)
		},
		Middleware: []http.MiddlewareFunc{
			middleware.RequestID(),
			middleware.Recover(app.Logger()),
			middleware.Logger(app.Logger()),
			middleware.CORS(),
		},
	}
	app.Register(routeProvider)

	// Bootstrap the application
	if err := app.Boot(); err != nil {
		log.Fatal("Failed to boot application:", err)
	}

	// Get the HTTP kernel from the route provider
	kernel := routeProvider.Kernel()

	// Get port from environment or default
	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}

	// Run the server with graceful shutdown
	app.Logger().Info("Starting server", "port", port, "env", app.Environment())

	if err := kernel.RunWithGracefulShutdown(":"+port, 10); err != nil {
		log.Fatal("Server error:", err)
	}
}

func registerRoutes(r *http.Router, app *foundation.Application) {
	// Create controllers
	userController := controllers.NewUserController(app)

	// Welcome route
	r.GET("/", func(ctx *http.Context) error {
		return ctx.JSONResponse(map[string]any{
			"message": "Welcome to Go-Genesys Example!",
			"version": app.Version(),
			"env":     app.Environment(),
		})
	}).Name("home")

	// Health check
	r.GET("/health", func(ctx *http.Context) error {
		return ctx.JSONResponse(map[string]any{
			"status":  "healthy",
			"version": app.Version(),
		})
	}).Name("health")

	// API v1 routes
	r.Group("/api/v1", func(api *http.Router) {
		// Users resource
		api.GET("/users", userController.Index).Name("api.users.index")
		api.POST("/users", userController.Store).Name("api.users.store")
		api.GET("/users/:id", userController.Show).Name("api.users.show")
		api.PUT("/users/:id", userController.Update).Name("api.users.update")
		api.DELETE("/users/:id", userController.Destroy).Name("api.users.destroy")

		// Example validation route
		api.POST("/validate", func(ctx *http.Context) error {
			type CreateUserRequest struct {
				Name  string `json:"name" validate:"required,min=2,max=100"`
				Email string `json:"email" validate:"required,email"`
				Age   int    `json:"age" validate:"required,gte=18,lte=120"`
			}

			var req CreateUserRequest
			if err := ctx.Bind(&req); err != nil {
				return ctx.BadRequest("Invalid JSON body")
			}

			// Get validator - simple approach without container lookup
			validator := validation.New()
			result := validator.Validate(&req)
			if result.Fails() {
				return ctx.Status(422).JSONResponse(map[string]any{
					"success": false,
					"message": "Validation failed",
					"errors":  result.Messages(),
				})
			}

			return ctx.JSONResponse(map[string]any{
				"success": true,
				"message": "Validation passed",
				"data":    req,
			})
		}).Name("api.validate")
	})

	// Static files (if exists)
	if _, err := os.Stat("public"); err == nil {
		r.Static("/public", "public")
	}
}
