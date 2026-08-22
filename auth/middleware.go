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
			// Resolve the user once per request so handlers can reach it
			// with ctx.User() instead of hitting the session or token
			// store again.
			if user := guard.User(ctx); user != nil {
				ctx.SetUser(user)
			}
			return next()
		}
		if opts.RedirectTo != "" && !ctx.IsJSON() && !ctx.IsAjax() {
			// Remember where they were headed, so the application can
			// send them on after they sign in - ctx.Intended(...), which
			// checks the destination rather than trusting it.
			if ctx.Method() == "GET" {
				ctx.SetIntendedURL(ctx.FiberCtx().OriginalURL())
			}
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

// ResolveUser puts the authenticated user on the context when there is
// one, and lets the request through when there is not.
//
// It is Laravel's default posture, where the guard knows who you are on
// every page rather than only on the guarded ones: a public page can
// greet a reader by name, show an edit link, or let a policy see a draft
// its author is entitled to. Register it globally, before the routes:
//
//	router.Use(auth.ResolveUser(guard))
//
// Guarding a route is still Middleware's job. A user another middleware
// already resolved is left alone, so this composes with a test's acting
// user or with a second guard.
func ResolveUser(guard Guard) http.MiddlewareFunc {
	return func(ctx *http.Context, next func() error) error {
		if !ctx.HasUser() && guard.Check(ctx) {
			if user := guard.User(ctx); user != nil {
				ctx.SetUser(user)
			}
		}
		return next()
	}
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
