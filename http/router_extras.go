package http

import (
	"github.com/gofiber/fiber/v2"
)

// Fallback registers a handler for requests that match no route. Register
// it after all other routes; it replaces the default 404 response.
func (r *Router) Fallback(handler HandlerFunc, middleware ...MiddlewareFunc) {
	r.fiber.Use(r.wrapHandler(handler, middleware...))
}

// Redirect registers a route that redirects to another location
// (302 by default):
//
//	router.Redirect("/old-dashboard", "/dashboard")
//	router.Redirect("/legacy", "/new", 301)
func (r *Router) Redirect(from, to string, status ...int) {
	code := fiber.StatusFound
	if len(status) > 0 {
		code = status[0]
	}
	handler := func(ctx *Context) error {
		return ctx.Redirect(to, code)
	}
	r.GET(from, handler)
	r.HEAD(from, handler)
}
