package routes

import (
	"github.com/genesysflow/go-genesys/facades/storage"
	"github.com/genesysflow/go-genesys/http"
)

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
	// Filesystem test
	r.GET("/filesystem/test", func(ctx *http.Context) error {
		// Assuming context comes from fiber context context
		c := ctx.FiberCtx().Context() // userContext
		if err := storage.Put(c, "test.txt", "Hello Filesystem!"); err != nil {
			return err
		}
		content, err := storage.Get(c, "test.txt")
		if err != nil {
			return err
		}
		return ctx.JSONResponse(map[string]any{
			"content": content,
			"exists":  storage.Exists(c, "test.txt"),
			"path":    storage.Url("test.txt"),
		})
	})
}
