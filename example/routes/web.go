package routes

import "github.com/genesysflow/go-genesys/http"

// Web registers web routes.
func Web(r *http.Router) {
	// Welcome route
	r.GET("/", func(ctx *http.Context) error {
		return ctx.JSONResponse(map[string]any{
			"message": "Welcome to Go-Genesys Example!",
			"version": "1.0.0",
		})
	}).Name("home")

	// Health check
	r.GET("/health", func(ctx *http.Context) error {
		return ctx.JSONResponse(map[string]any{
			"status": "healthy",
		})
	}).Name("health")
}
