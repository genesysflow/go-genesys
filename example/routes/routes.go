package routes

import (
	"github.com/genesysflow/go-genesys/foundation"
	"github.com/genesysflow/go-genesys/http"
	"github.com/genesysflow/go-genesys/http/middleware"
)

// GlobalMiddleware returns the global middleware stack.
func GlobalMiddleware(app *foundation.Application) []http.MiddlewareFunc {
	return []http.MiddlewareFunc{
		middleware.RequestID(),
		middleware.Logger(app.GetLogger()),
		middleware.Recover(app.GetLogger()),
		middleware.CORS(),
	}
}

// Register registers all application routes.
func Register(r *http.Router) {
	// Load web routes
	Web(r)

	// Load API routes
	API(r)
}
