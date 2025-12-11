package http

import (
	"sync"

	"github.com/genesysflow/go-genesys/contracts"
	"github.com/gofiber/fiber/v2"
)

// Context represents the request context passed to handlers.
// It wraps both Request and Response for convenient access.
type Context struct {
	request  *Request
	response *Response
	app      contracts.Application
	fiberCtx *fiber.Ctx
	store    sync.Map
	aborted  bool
	next     func() error
}

// NewContext creates a new Context.
func NewContext(fiberCtx *fiber.Ctx, app contracts.Application) *Context {
	return &Context{
		request:  NewRequest(fiberCtx),
		response: NewResponse(fiberCtx),
		app:      app,
		fiberCtx: fiberCtx,
	}
}

// Request returns the HTTP request.
func (c *Context) Request() *Request {
	return c.request
}

// Response returns the HTTP response.
func (c *Context) Response() *Response {
	return c.response
}

// App returns the application instance.
func (c *Context) App() contracts.Application {
	return c.app
}

// FiberCtx returns the underlying Fiber context.
func (c *Context) FiberCtx() *fiber.Ctx {
	return c.fiberCtx
}

// Param returns a route parameter.
func (c *Context) Param(key string, defaultValue ...string) string {
	return c.request.Param(key, defaultValue...)
}

// ParamInt returns a route parameter as integer.
func (c *Context) ParamInt(key string, defaultValue ...int) int {
	return c.request.ParamInt(key, defaultValue...)
}

// Query returns a query parameter.
func (c *Context) Query(key string, defaultValue ...string) string {
	return c.request.Query(key, defaultValue...)
}

// QueryInt returns a query parameter as integer.
func (c *Context) QueryInt(key string, defaultValue ...int) int {
	return c.request.QueryInt(key, defaultValue...)
}

// Input returns an input value.
func (c *Context) Input(key string, defaultValue ...string) string {
	return c.request.Input(key, defaultValue...)
}

// All returns all input data.
func (c *Context) All() map[string]any {
	return c.request.All()
}

// JSON parses the request body as JSON.
func (c *Context) JSON(v any) error {
	return c.request.JSON(v)
}

// Bind binds the request body to a struct (alias for JSON).
func (c *Context) Bind(v any) error {
	return c.fiberCtx.BodyParser(v)
}

// Validate validates the request data against the given rules.
// This is a placeholder - actual validation is implemented in the validation package.
func (c *Context) Validate(v any) error {
	// Validation would be implemented using the validation package
	return nil
}

// Get retrieves a value from the context store.
func (c *Context) Get(key string) any {
	value, _ := c.store.Load(key)
	return value
}

// Set stores a value in the context store.
func (c *Context) Set(key string, value any) {
	c.store.Store(key, value)
}

// SetNext sets the next handler function for middleware.
func (c *Context) SetNext(next func() error) {
	c.next = next
}

// Next calls the next middleware in the chain.
func (c *Context) Next() error {
	if c.next != nil {
		return c.next()
	}
	return c.fiberCtx.Next()
}

// Abort aborts the middleware chain.
func (c *Context) Abort() error {
	c.aborted = true
	return nil
}

// IsAborted returns true if the context has been aborted.
func (c *Context) IsAborted() bool {
	return c.aborted
}

// AbortWithStatus aborts with a status code.
func (c *Context) AbortWithStatus(code int) error {
	c.aborted = true
	return c.fiberCtx.SendStatus(code)
}

// AbortWithJSON aborts with a JSON response.
func (c *Context) AbortWithJSON(code int, v any) error {
	c.aborted = true
	c.fiberCtx.Status(code)
	return c.fiberCtx.JSON(v)
}

// AbortWithError aborts with an error.
func (c *Context) AbortWithError(code int, err error) error {
	c.aborted = true
	c.fiberCtx.Status(code)
	return c.fiberCtx.JSON(fiber.Map{
		"error": err.Error(),
	})
}

// Status sets the response status code.
func (c *Context) Status(code int) *Context {
	c.fiberCtx.Status(code)
	return c
}

// Header sets a response header.
func (c *Context) Header(key, value string) *Context {
	c.fiberCtx.Set(key, value)
	return c
}

// String sends a string response.
func (c *Context) String(s string) error {
	return c.fiberCtx.SendString(s)
}

// JSONResponse sends a JSON response.
func (c *Context) JSONResponse(v any) error {
	return c.fiberCtx.JSON(v)
}

// HTML sends an HTML response.
func (c *Context) HTML(html string) error {
	c.fiberCtx.Set("Content-Type", "text/html; charset=utf-8")
	return c.fiberCtx.SendString(html)
}

// File sends a file response.
func (c *Context) File(path string) error {
	return c.fiberCtx.SendFile(path)
}

// Download sends a file as download.
func (c *Context) Download(path string, filename ...string) error {
	if len(filename) > 0 {
		return c.fiberCtx.Download(path, filename[0])
	}
	return c.fiberCtx.Download(path)
}

// Redirect redirects to another URL.
func (c *Context) Redirect(url string, status ...int) error {
	code := fiber.StatusFound
	if len(status) > 0 {
		code = status[0]
	}
	return c.fiberCtx.Redirect(url, code)
}

// RedirectBack redirects to the previous page.
func (c *Context) RedirectBack(fallback ...string) error {
	return c.response.RedirectBack(fallback...)
}

// NoContent sends a 204 No Content response.
func (c *Context) NoContent() error {
	return c.fiberCtx.SendStatus(fiber.StatusNoContent)
}

// Send sends bytes as response.
func (c *Context) Send(body []byte) error {
	return c.fiberCtx.Send(body)
}

// Created sends a 201 Created response with JSON body.
func (c *Context) Created(v any) error {
	c.fiberCtx.Status(fiber.StatusCreated)
	return c.fiberCtx.JSON(v)
}

// Accepted sends a 202 Accepted response.
func (c *Context) Accepted(v ...any) error {
	c.fiberCtx.Status(fiber.StatusAccepted)
	if len(v) > 0 {
		return c.fiberCtx.JSON(v[0])
	}
	return nil
}

// BadRequest sends a 400 Bad Request response.
func (c *Context) BadRequest(message ...string) error {
	c.fiberCtx.Status(fiber.StatusBadRequest)
	msg := "Bad Request"
	if len(message) > 0 {
		msg = message[0]
	}
	return c.fiberCtx.JSON(fiber.Map{"error": msg})
}

// Unauthorized sends a 401 Unauthorized response.
func (c *Context) Unauthorized(message ...string) error {
	c.fiberCtx.Status(fiber.StatusUnauthorized)
	msg := "Unauthorized"
	if len(message) > 0 {
		msg = message[0]
	}
	return c.fiberCtx.JSON(fiber.Map{"error": msg})
}

// Forbidden sends a 403 Forbidden response.
func (c *Context) Forbidden(message ...string) error {
	c.fiberCtx.Status(fiber.StatusForbidden)
	msg := "Forbidden"
	if len(message) > 0 {
		msg = message[0]
	}
	return c.fiberCtx.JSON(fiber.Map{"error": msg})
}

// NotFound sends a 404 Not Found response.
func (c *Context) NotFound(message ...string) error {
	c.fiberCtx.Status(fiber.StatusNotFound)
	msg := "Not Found"
	if len(message) > 0 {
		msg = message[0]
	}
	return c.fiberCtx.JSON(fiber.Map{"error": msg})
}

// InternalServerError sends a 500 Internal Server Error response.
func (c *Context) InternalServerError(message ...string) error {
	c.fiberCtx.Status(fiber.StatusInternalServerError)
	msg := "Internal Server Error"
	if len(message) > 0 {
		msg = message[0]
	}
	return c.fiberCtx.JSON(fiber.Map{"error": msg})
}

// Cookie sets a cookie.
func (c *Context) Cookie(cookie *Cookie) *Context {
	c.response.Cookie(cookie)
	return c
}

// ClearCookie clears a cookie.
func (c *Context) ClearCookie(name string) *Context {
	c.response.ClearCookie(name)
	return c
}

// IP returns the client IP address.
func (c *Context) IP() string {
	return c.request.IP()
}

// Method returns the HTTP method.
func (c *Context) Method() string {
	return c.request.Method()
}

// Path returns the request path.
func (c *Context) Path() string {
	return c.request.Path()
}

// IsAjax returns true if this is an AJAX request.
func (c *Context) IsAjax() bool {
	return c.request.IsAjax()
}

// IsJSON returns true if the request wants JSON response.
func (c *Context) IsJSON() bool {
	return c.request.IsJSON()
}

