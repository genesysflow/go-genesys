package http

import (
	"encoding/json"
	"encoding/xml"
	"io"

	"github.com/gofiber/fiber/v2"
)

// Response wraps Fiber's response to provide a Laravel-like API.
type Response struct {
	ctx  *fiber.Ctx
	sent bool
}

// NewResponse creates a new Response wrapper.
func NewResponse(ctx *fiber.Ctx) *Response {
	return &Response{
		ctx: ctx,
	}
}

// FiberCtx returns the underlying Fiber context.
func (r *Response) FiberCtx() *fiber.Ctx {
	return r.ctx
}

// Status sets the HTTP status code.
func (r *Response) Status(code int) *Response {
	r.ctx.Status(code)
	return r
}

// Header sets a response header.
func (r *Response) Header(key, value string) *Response {
	r.ctx.Set(key, value)
	return r
}

// Headers sets multiple response headers.
func (r *Response) Headers(headers map[string]string) *Response {
	for key, value := range headers {
		r.ctx.Set(key, value)
	}
	return r
}

// Cookie sets a cookie.
func (r *Response) Cookie(cookie *Cookie) *Response {
	r.ctx.Cookie(&fiber.Cookie{
		Name:     cookie.Name,
		Value:    cookie.Value,
		Path:     cookie.Path,
		Domain:   cookie.Domain,
		MaxAge:   cookie.MaxAge,
		Secure:   cookie.Secure,
		HTTPOnly: cookie.HTTPOnly,
		SameSite: cookie.SameSite,
	})
	return r
}

// ClearCookie clears a cookie.
func (r *Response) ClearCookie(name string) *Response {
	r.ctx.ClearCookie(name)
	return r
}

// Body sends a response body.
func (r *Response) Body(body []byte) error {
	r.sent = true
	return r.ctx.Send(body)
}

// String sends a string response.
func (r *Response) String(s string) error {
	r.sent = true
	return r.ctx.SendString(s)
}

// JSON sends a JSON response.
func (r *Response) JSON(v any) error {
	r.sent = true
	return r.ctx.JSON(v)
}

// PrettyJSON sends a pretty-printed JSON response.
func (r *Response) PrettyJSON(v any) error {
	r.sent = true
	r.ctx.Set("Content-Type", "application/json")
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return r.ctx.Send(data)
}

// JSONP sends a JSONP response.
func (r *Response) JSONP(v any, callback string) error {
	r.sent = true
	return r.ctx.JSONP(v, callback)
}

// XML sends an XML response.
func (r *Response) XML(v any) error {
	r.sent = true
	r.ctx.Set("Content-Type", "application/xml")
	data, err := xml.Marshal(v)
	if err != nil {
		return err
	}
	return r.ctx.Send(data)
}

// HTML sends an HTML response.
func (r *Response) HTML(html string) error {
	r.sent = true
	r.ctx.Set("Content-Type", "text/html; charset=utf-8")
	return r.ctx.SendString(html)
}

// File sends a file response.
func (r *Response) File(path string) error {
	r.sent = true
	return r.ctx.SendFile(path)
}

// Download sends a file as download.
func (r *Response) Download(path string, filename ...string) error {
	r.sent = true
	if len(filename) > 0 {
		return r.ctx.Download(path, filename[0])
	}
	return r.ctx.Download(path)
}

// Redirect redirects to another URL.
func (r *Response) Redirect(url string, status ...int) error {
	r.sent = true
	code := fiber.StatusFound
	if len(status) > 0 {
		code = status[0]
	}
	return r.ctx.Redirect(url, code)
}

// RedirectBack redirects to the previous page.
func (r *Response) RedirectBack(fallback ...string) error {
	r.sent = true
	referer := r.ctx.Get("Referer")
	if referer == "" {
		if len(fallback) > 0 {
			return r.ctx.Redirect(fallback[0])
		}
		return r.ctx.Redirect("/")
	}
	return r.ctx.Redirect(referer)
}

// RedirectRoute redirects to a named route.
// Note: This requires the router to support named routes.
func (r *Response) RedirectRoute(name string, params ...map[string]any) error {
	r.sent = true
	// For now, redirect to the name as a path
	// Full implementation would look up the route by name
	return r.ctx.Redirect("/" + name)
}

// NoContent sends a 204 No Content response.
func (r *Response) NoContent() error {
	r.sent = true
	return r.ctx.SendStatus(fiber.StatusNoContent)
}

// Stream streams a response.
func (r *Response) Stream(contentType string, reader io.Reader) error {
	r.sent = true
	r.ctx.Set("Content-Type", contentType)
	return r.ctx.SendStream(reader)
}

// Write writes bytes to the response.
func (r *Response) Write(p []byte) (int, error) {
	r.sent = true
	return r.ctx.Write(p)
}

// Flush flushes the response.
// Note: Fiber handles this automatically in most cases.
func (r *Response) Flush() error {
	return nil
}

// Sent returns true if the response has been sent.
func (r *Response) Sent() bool {
	return r.sent
}

// Type sets the Content-Type header based on file extension or MIME type.
func (r *Response) Type(contentType string) *Response {
	r.ctx.Type(contentType)
	return r
}

// Vary adds a Vary header.
func (r *Response) Vary(headers ...string) *Response {
	for _, h := range headers {
		r.ctx.Vary(h)
	}
	return r
}

// Append appends a value to a header.
func (r *Response) Append(key, value string) *Response {
	r.ctx.Append(key, value)
	return r
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

// NewCookie creates a new Cookie with defaults.
func NewCookie(name, value string) *Cookie {
	return &Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		HTTPOnly: true,
		SameSite: "Lax",
	}
}

// WithPath sets the cookie path.
func (c *Cookie) WithPath(path string) *Cookie {
	c.Path = path
	return c
}

// WithDomain sets the cookie domain.
func (c *Cookie) WithDomain(domain string) *Cookie {
	c.Domain = domain
	return c
}

// WithMaxAge sets the cookie max age.
func (c *Cookie) WithMaxAge(maxAge int) *Cookie {
	c.MaxAge = maxAge
	return c
}

// WithSecure sets the cookie secure flag.
func (c *Cookie) WithSecure(secure bool) *Cookie {
	c.Secure = secure
	return c
}

// WithHTTPOnly sets the cookie HTTPOnly flag.
func (c *Cookie) WithHTTPOnly(httpOnly bool) *Cookie {
	c.HTTPOnly = httpOnly
	return c
}

// WithSameSite sets the cookie SameSite attribute.
func (c *Cookie) WithSameSite(sameSite string) *Cookie {
	c.SameSite = sameSite
	return c
}

