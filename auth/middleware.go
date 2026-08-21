package auth

import (
	"github.com/genesysflow/go-genesys/errors"
	"github.com/genesysflow/go-genesys/http"
)

// Middleware rejects unauthenticated requests with 401 (JSON requests) or
// a redirect to the login page (browser requests, when RedirectTo is set).
func Middleware(guard Guard, options ...MiddlewareOptions) http.MiddlewareFunc {
	opts := MiddlewareOptions{}
	if len(options) > 0 {
		opts = options[0]
	}
	return func(ctx *http.Context, next func() error) error {
		if guard.Check(ctx) {
			return next()
		}
		if opts.RedirectTo != "" && !ctx.IsJSON() && !ctx.IsAjax() {
			return ctx.Redirect(opts.RedirectTo)
		}
		return errors.Unauthorized("Unauthenticated.")
	}
}

// MiddlewareOptions configures the auth middleware.
type MiddlewareOptions struct {
	// RedirectTo sends unauthenticated browser requests to this path
	// (e.g. "/login") instead of returning 401.
	RedirectTo string
}

// GuestMiddleware redirects authenticated users away (e.g. from login
// pages) to the given path.
func GuestMiddleware(guard Guard, redirectTo string) http.MiddlewareFunc {
	return func(ctx *http.Context, next func() error) error {
		if guard.Check(ctx) {
			return ctx.Redirect(redirectTo)
		}
		return next()
	}
}
