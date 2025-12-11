package contracts

// Router defines the interface for HTTP routing.
type Router interface {
	// GET registers a GET route.
	GET(path string, handler HandlerFunc, middleware ...MiddlewareFunc) Route

	// POST registers a POST route.
	POST(path string, handler HandlerFunc, middleware ...MiddlewareFunc) Route

	// PUT registers a PUT route.
	PUT(path string, handler HandlerFunc, middleware ...MiddlewareFunc) Route

	// PATCH registers a PATCH route.
	PATCH(path string, handler HandlerFunc, middleware ...MiddlewareFunc) Route

	// DELETE registers a DELETE route.
	DELETE(path string, handler HandlerFunc, middleware ...MiddlewareFunc) Route

	// OPTIONS registers an OPTIONS route.
	OPTIONS(path string, handler HandlerFunc, middleware ...MiddlewareFunc) Route

	// HEAD registers a HEAD route.
	HEAD(path string, handler HandlerFunc, middleware ...MiddlewareFunc) Route

	// Any registers a route for all HTTP methods.
	Any(path string, handler HandlerFunc, middleware ...MiddlewareFunc) Route

	// Match registers a route for specific HTTP methods.
	Match(methods []string, path string, handler HandlerFunc, middleware ...MiddlewareFunc) Route

	// Group creates a route group with shared attributes.
	Group(prefix string, fn func(Router), middleware ...MiddlewareFunc)

	// Use registers global middleware.
	Use(middleware ...MiddlewareFunc)

	// Static serves static files from a directory.
	Static(prefix, root string)

	// Routes returns all registered routes.
	Routes() []Route
}

// Route represents a single route.
type Route interface {
	// Name sets the route name.
	Name(name string) Route

	// Middleware adds middleware to the route.
	Middleware(middleware ...MiddlewareFunc) Route

	// GetName returns the route name.
	GetName() string

	// GetPath returns the route path.
	GetPath() string

	// GetMethod returns the route method.
	GetMethod() string

	// GetHandler returns the route handler.
	GetHandler() HandlerFunc
}

// HandlerFunc is the function signature for route handlers.
type HandlerFunc func(ctx Context) error

// MiddlewareFunc is the function signature for middleware.
type MiddlewareFunc func(ctx Context, next func() error) error

// Context represents the request context passed to handlers.
type Context interface {
	// Request returns the HTTP request.
	Request() Request

	// Response returns the HTTP response.
	Response() Response

	// App returns the application instance.
	App() Application

	// Param returns a route parameter.
	Param(key string, defaultValue ...string) string

	// ParamInt returns a route parameter as integer.
	ParamInt(key string, defaultValue ...int) int

	// Query returns a query parameter.
	Query(key string, defaultValue ...string) string

	// QueryInt returns a query parameter as integer.
	QueryInt(key string, defaultValue ...int) int

	// Input returns an input value.
	Input(key string, defaultValue ...string) string

	// All returns all input data.
	All() map[string]any

	// JSON parses the request body as JSON.
	JSON(v any) error

	// Validate validates the request data against the given rules.
	Validate(v any) error

	// Get retrieves a value from the context store.
	Get(key string) any

	// Set stores a value in the context store.
	Set(key string, value any)

	// Next calls the next middleware in the chain.
	Next() error

	// Abort aborts the middleware chain.
	Abort() error

	// AbortWithStatus aborts with a status code.
	AbortWithStatus(code int) error

	// AbortWithJSON aborts with a JSON response.
	AbortWithJSON(code int, v any) error

	// Status sets the response status code.
	Status(code int) Context

	// Header sets a response header.
	Header(key, value string) Context

	// String sends a string response.
	String(s string) error

	// JSONResponse sends a JSON response.
	JSONResponse(v any) error

	// HTML sends an HTML response.
	HTML(html string) error

	// File sends a file response.
	File(path string) error

	// Redirect redirects to another URL.
	Redirect(url string, status ...int) error

	// NoContent sends a 204 No Content response.
	NoContent() error
}
