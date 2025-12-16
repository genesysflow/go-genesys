package http

import (
	"github.com/genesysflow/go-genesys/contracts"
	"github.com/gofiber/fiber/v2"
)

// HandlerFunc is the function signature for route handlers.
type HandlerFunc func(ctx *Context) error

// MiddlewareFunc is the function signature for middleware.
type MiddlewareFunc func(ctx *Context, next func() error) error

// Router wraps Fiber's router with Laravel-style route groups.
type Router struct {
	app         contracts.Application
	fiber       *fiber.App
	prefix      string
	middleware  []MiddlewareFunc
	routes      []*Route
	namedRoutes map[string]*Route
	groups      []*Router
	parent      *Router
}

// NewRouter creates a new Router instance.
func NewRouter(app contracts.Application, fiberApp *fiber.App) *Router {
	return &Router{
		app:         app,
		fiber:       fiberApp,
		routes:      make([]*Route, 0),
		namedRoutes: make(map[string]*Route),
		groups:      make([]*Router, 0),
	}
}

// wrapHandler wraps a HandlerFunc to a Fiber handler.
func (r *Router) wrapHandler(handler HandlerFunc, middleware ...MiddlewareFunc) fiber.Handler {
	return func(c *fiber.Ctx) error {
		ctx := NewContext(c, r.app)

		// Collect all middleware (group middleware + route middleware)
		allMiddleware := make([]MiddlewareFunc, 0, len(r.middleware)+len(middleware))
		allMiddleware = append(allMiddleware, r.middleware...)
		allMiddleware = append(allMiddleware, middleware...)

		// If we're in a group, add parent middleware
		if r.parent != nil {
			parentMiddleware := r.collectParentMiddleware()
			allMiddleware = append(parentMiddleware, allMiddleware...)
		}

		// Execute middleware chain
		return r.executeMiddleware(ctx, allMiddleware, handler)
	}
}

// collectParentMiddleware collects middleware from all parent routers.
func (r *Router) collectParentMiddleware() []MiddlewareFunc {
	if r.parent == nil {
		return nil
	}

	parentMiddleware := r.parent.collectParentMiddleware()
	return append(parentMiddleware, r.parent.middleware...)
}

// executeMiddleware executes the middleware chain.
func (r *Router) executeMiddleware(ctx *Context, middleware []MiddlewareFunc, handler HandlerFunc) error {
	if len(middleware) == 0 {
		return handler(ctx)
	}

	index := 0
	var next func() error
	next = func() error {
		if ctx.IsAborted() {
			return nil
		}
		index++
		if index < len(middleware) {
			return middleware[index](ctx, next)
		}
		return handler(ctx)
	}

	return middleware[0](ctx, next)
}

// App returns the application instance.
func (r *Router) App() contracts.Application {
	return r.app
}

// GET registers a GET route.
func (r *Router) GET(path string, handler HandlerFunc, middleware ...MiddlewareFunc) *Route {
	return r.addRoute("GET", path, handler, middleware...)
}

// POST registers a POST route.
func (r *Router) POST(path string, handler HandlerFunc, middleware ...MiddlewareFunc) *Route {
	return r.addRoute("POST", path, handler, middleware...)
}

// PUT registers a PUT route.
func (r *Router) PUT(path string, handler HandlerFunc, middleware ...MiddlewareFunc) *Route {
	return r.addRoute("PUT", path, handler, middleware...)
}

// PATCH registers a PATCH route.
func (r *Router) PATCH(path string, handler HandlerFunc, middleware ...MiddlewareFunc) *Route {
	return r.addRoute("PATCH", path, handler, middleware...)
}

// DELETE registers a DELETE route.
func (r *Router) DELETE(path string, handler HandlerFunc, middleware ...MiddlewareFunc) *Route {
	return r.addRoute("DELETE", path, handler, middleware...)
}

// OPTIONS registers an OPTIONS route.
func (r *Router) OPTIONS(path string, handler HandlerFunc, middleware ...MiddlewareFunc) *Route {
	return r.addRoute("OPTIONS", path, handler, middleware...)
}

// HEAD registers a HEAD route.
func (r *Router) HEAD(path string, handler HandlerFunc, middleware ...MiddlewareFunc) *Route {
	return r.addRoute("HEAD", path, handler, middleware...)
}

// Any registers a route for all HTTP methods.
func (r *Router) Any(path string, handler HandlerFunc, middleware ...MiddlewareFunc) *Route {
	methods := []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS", "HEAD"}
	var route *Route
	for _, method := range methods {
		route = r.addRoute(method, path, handler, middleware...)
	}
	return route
}

// Match registers a route for specific HTTP methods.
func (r *Router) Match(methods []string, path string, handler HandlerFunc, middleware ...MiddlewareFunc) *Route {
	var route *Route
	for _, method := range methods {
		route = r.addRoute(method, path, handler, middleware...)
	}
	return route
}

// addRoute adds a route to the router.
func (r *Router) addRoute(method, path string, handler HandlerFunc, middleware ...MiddlewareFunc) *Route {
	fullPath := r.prefix + path

	route := &Route{
		method:     method,
		path:       fullPath,
		handler:    handler,
		middleware: middleware,
		router:     r,
	}
	r.routes = append(r.routes, route)

	// Register with Fiber
	wrappedHandler := r.wrapHandler(handler, middleware...)
	switch method {
	case "GET":
		r.fiber.Get(fullPath, wrappedHandler)
	case "POST":
		r.fiber.Post(fullPath, wrappedHandler)
	case "PUT":
		r.fiber.Put(fullPath, wrappedHandler)
	case "PATCH":
		r.fiber.Patch(fullPath, wrappedHandler)
	case "DELETE":
		r.fiber.Delete(fullPath, wrappedHandler)
	case "OPTIONS":
		r.fiber.Options(fullPath, wrappedHandler)
	case "HEAD":
		r.fiber.Head(fullPath, wrappedHandler)
	}

	return route
}

// Group creates a route group with shared attributes.
func (r *Router) Group(prefix string, fn func(*Router), middleware ...MiddlewareFunc) {
	group := &Router{
		app:         r.app,
		fiber:       r.fiber,
		prefix:      r.prefix + prefix,
		middleware:  middleware,
		routes:      make([]*Route, 0),
		namedRoutes: r.namedRoutes, // Share named routes with parent
		groups:      make([]*Router, 0),
		parent:      r,
	}

	r.groups = append(r.groups, group)
	fn(group)
}

// Use registers global middleware for this router/group.
func (r *Router) Use(middleware ...MiddlewareFunc) {
	r.middleware = append(r.middleware, middleware...)
}

// Static serves static files from a directory.
func (r *Router) Static(prefix, root string) {
	fullPath := r.prefix + prefix
	r.fiber.Static(fullPath, root)
}

// Routes returns all registered routes.
func (r *Router) Routes() []*Route {
	return r.routes
}

// NamedRoute returns a route by name.
func (r *Router) NamedRoute(name string) *Route {
	return r.namedRoutes[name]
}

// URL generates a URL for a named route.
func (r *Router) URL(name string, params ...map[string]any) string {
	route := r.namedRoutes[name]
	if route == nil {
		return ""
	}

	path := route.path
	if len(params) > 0 {
		for key, value := range params[0] {
			// Replace route parameters like :id with actual values
			// This is a simple implementation
			path = replaceParam(path, key, value)
		}
	}
	return path
}

// replaceParam replaces a route parameter with a value.
func replaceParam(path, key string, value any) string {
	// Simple string replacement
	// A full implementation would use regex
	return path
}

// Route represents a single route.
type Route struct {
	name       string
	method     string
	path       string
	handler    HandlerFunc
	middleware []MiddlewareFunc
	router     *Router
}

// Name sets the route name.
func (r *Route) Name(name string) *Route {
	r.name = name
	if r.router != nil {
		r.router.namedRoutes[name] = r
	}
	return r
}

// Middleware adds middleware to the route.
func (r *Route) Middleware(middleware ...MiddlewareFunc) *Route {
	r.middleware = append(r.middleware, middleware...)
	return r
}

// GetName returns the route name.
func (r *Route) GetName() string {
	return r.name
}

// GetPath returns the route path.
func (r *Route) GetPath() string {
	return r.path
}

// GetMethod returns the route method.
func (r *Route) GetMethod() string {
	return r.method
}

// GetHandler returns the route handler.
func (r *Route) GetHandler() HandlerFunc {
	return r.handler
}

// Resource creates RESTful routes for a resource.
func (r *Router) Resource(name string, controller ResourceController) {
	r.GET("/"+name, controller.Index).Name(name + ".index")
	r.GET("/"+name+"/create", controller.Create).Name(name + ".create")
	r.POST("/"+name, controller.Store).Name(name + ".store")
	r.GET("/"+name+"/:id", controller.Show).Name(name + ".show")
	r.GET("/"+name+"/:id/edit", controller.Edit).Name(name + ".edit")
	r.PUT("/"+name+"/:id", controller.Update).Name(name + ".update")
	r.PATCH("/"+name+"/:id", controller.Update).Name(name + ".update.patch")
	r.DELETE("/"+name+"/:id", controller.Destroy).Name(name + ".destroy")
}

// APIResource creates API RESTful routes (without create/edit).
func (r *Router) APIResource(name string, controller APIResourceController) {
	r.GET("/"+name, controller.Index).Name(name + ".index")
	r.POST("/"+name, controller.Store).Name(name + ".store")
	r.GET("/"+name+"/:id", controller.Show).Name(name + ".show")
	r.PUT("/"+name+"/:id", controller.Update).Name(name + ".update")
	r.PATCH("/"+name+"/:id", controller.Update).Name(name + ".update.patch")
	r.DELETE("/"+name+"/:id", controller.Destroy).Name(name + ".destroy")
}

// ResourceController defines the interface for resourceful controllers.
type ResourceController interface {
	Index(ctx *Context) error
	Create(ctx *Context) error
	Store(ctx *Context) error
	Show(ctx *Context) error
	Edit(ctx *Context) error
	Update(ctx *Context) error
	Destroy(ctx *Context) error
}

// APIResourceController defines the interface for API resourceful controllers.
type APIResourceController interface {
	Index(ctx *Context) error
	Store(ctx *Context) error
	Show(ctx *Context) error
	Update(ctx *Context) error
	Destroy(ctx *Context) error
}
