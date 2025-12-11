package contracts

import (
	"context"
	"io"
	"mime/multipart"
)

// Request defines the interface for HTTP requests.
type Request interface {
	// Context returns the request context.
	Context() context.Context

	// WithContext returns a copy of the request with the given context.
	WithContext(ctx context.Context) Request

	// Method returns the HTTP method.
	Method() string

	// URI returns the request URI.
	URI() string

	// Path returns the request path.
	Path() string

	// FullURL returns the full URL including query string.
	FullURL() string

	// Host returns the host.
	Host() string

	// Scheme returns the scheme (http or https).
	Scheme() string

	// IsSecure returns true if the request is using HTTPS.
	IsSecure() bool

	// IP returns the client IP address.
	IP() string

	// IPs returns all client IP addresses from X-Forwarded-For.
	IPs() []string

	// Header returns a header value.
	Header(key string) string

	// Headers returns all headers.
	Headers() map[string]string

	// Query returns a query string parameter.
	Query(key string, defaultValue ...string) string

	// QueryInt returns a query parameter as integer.
	QueryInt(key string, defaultValue ...int) int

	// Param returns a route parameter.
	Param(key string, defaultValue ...string) string

	// ParamInt returns a route parameter as integer.
	ParamInt(key string, defaultValue ...int) int

	// Input returns a request input value (from body, query, or params).
	Input(key string, defaultValue ...string) string

	// All returns all input data.
	All() map[string]any

	// Only returns only the specified input keys.
	Only(keys ...string) map[string]any

	// Except returns all input except the specified keys.
	Except(keys ...string) map[string]any

	// Has checks if an input key exists.
	Has(key string) bool

	// Filled checks if an input key exists and is not empty.
	Filled(key string) bool

	// Body returns the raw request body.
	Body() []byte

	// BodyReader returns an io.Reader for the request body.
	BodyReader() io.Reader

	// JSON parses the JSON body into the given struct.
	JSON(v any) error

	// File returns an uploaded file.
	File(key string) (*multipart.FileHeader, error)

	// Files returns all uploaded files for a key.
	Files(key string) ([]*multipart.FileHeader, error)

	// Cookie returns a cookie value.
	Cookie(key string) string

	// Cookies returns all cookies.
	Cookies() map[string]string

	// IsAjax returns true if this is an AJAX request.
	IsAjax() bool

	// IsJSON returns true if the request wants JSON response.
	IsJSON() bool

	// Accepts checks if the request accepts the given content type.
	Accepts(contentType string) bool

	// Get retrieves a value from the request store.
	Get(key string) any

	// Set stores a value in the request store.
	Set(key string, value any)
}

// Response defines the interface for HTTP responses.
type Response interface {
	// Status sets the HTTP status code.
	Status(code int) Response

	// Header sets a response header.
	Header(key, value string) Response

	// Headers sets multiple response headers.
	Headers(headers map[string]string) Response

	// Cookie sets a cookie.
	Cookie(cookie *Cookie) Response

	// ClearCookie clears a cookie.
	ClearCookie(name string) Response

	// Body sends a response body.
	Body(body []byte) error

	// String sends a string response.
	String(s string) error

	// JSON sends a JSON response.
	JSON(v any) error

	// JSONP sends a JSONP response.
	JSONP(v any, callback string) error

	// XML sends an XML response.
	XML(v any) error

	// HTML sends an HTML response.
	HTML(html string) error

	// File sends a file response.
	File(path string) error

	// Download sends a file as download.
	Download(path string, filename ...string) error

	// Redirect redirects to another URL.
	Redirect(url string, status ...int) error

	// RedirectBack redirects to the previous page.
	RedirectBack(fallback ...string) error

	// RedirectRoute redirects to a named route.
	RedirectRoute(name string, params ...map[string]any) error

	// NoContent sends a 204 No Content response.
	NoContent() error

	// Stream streams a response.
	Stream(contentType string, reader io.Reader) error

	// Write writes bytes to the response.
	Write(p []byte) (int, error)

	// Flush flushes the response.
	Flush() error

	// Sent returns true if the response has been sent.
	Sent() bool
}

// Cookie represents an HTTP cookie.
type Cookie struct {
	Name     string
	Value    string
	Path     string
	Domain   string
	MaxAge   int
	Secure   bool
	HTTPOnly bool
	SameSite string
}
