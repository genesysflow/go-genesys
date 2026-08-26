package http

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/genesysflow/go-genesys/contracts"
	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// serveResponse drives a GET / through a handler holding the framework's
// Response wrapper, and returns the response together with its body.
func serveResponse(t *testing.T, handler func(*Response) error) (*http.Response, string) {
	t.Helper()

	app := newTestApp()
	app.Get("/", func(c *fiber.Ctx) error {
		return handler(NewResponse(c))
	})

	resp, err := app.Test(httptest.NewRequest("GET", "/", nil), -1)
	require.NoError(t, err)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())

	return resp, string(body)
}

// --- JSONP ---

// JSONP wraps the payload in the caller's callback and declares itself
// JavaScript, so a script tag can consume it.
func TestResponseJSONP(t *testing.T) {
	resp, body := serveResponse(t, func(r *Response) error {
		return r.JSONP(map[string]any{"name": "Ada", "id": 7}, "handleUsers")
	})

	assert.Equal(t, 200, resp.StatusCode)
	assert.True(t, strings.HasPrefix(body, "handleUsers("), "body was %q", body)
	assert.True(t, strings.HasSuffix(body, ");"), "body was %q", body)
	assert.Contains(t, body, `"name":"Ada"`)
	assert.Contains(t, resp.Header.Get("Content-Type"), "javascript")
	assert.Equal(t, "nosniff", resp.Header.Get("X-Content-Type-Options"))
}

// A payload that cannot be marshalled surfaces the error instead of
// sending half a response.
func TestResponseJSONPMarshalError(t *testing.T) {
	app := newTestApp()
	var sendErr error
	app.Get("/", func(c *fiber.Ctx) error {
		sendErr = NewResponse(c).JSONP(make(chan int), "cb")
		return c.SendString("handled")
	})

	_, err := app.Test(httptest.NewRequest("GET", "/", nil), -1)
	require.NoError(t, err)
	assert.Error(t, sendErr)
}

// --- File and Download ---

// tempFile writes a file into the test's own directory and returns its path.
func tempFile(t *testing.T, name, contents string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.WriteFile(path, []byte(contents), 0o600))
	return path
}

// File serves the bytes inline, with no Content-Disposition.
func TestResponseFile(t *testing.T) {
	path := tempFile(t, "notes.txt", "the quick brown fox")

	resp, body := serveResponse(t, func(r *Response) error {
		return r.File(path)
	})

	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, "the quick brown fox", body)
	assert.Empty(t, resp.Header.Get("Content-Disposition"))
}

// A path that does not exist is a 404, not a panic.
func TestResponseFileMissing(t *testing.T) {
	resp, _ := serveResponse(t, func(r *Response) error {
		return r.File(filepath.Join(t.TempDir(), "absent.txt"))
	})

	assert.Equal(t, 404, resp.StatusCode)
}

// Download names the attachment after the file on disk.
func TestResponseDownloadUsesFileName(t *testing.T) {
	path := tempFile(t, "report.csv", "id,name\n1,Ada\n")

	resp, body := serveResponse(t, func(r *Response) error {
		return r.Download(path)
	})

	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, "id,name\n1,Ada\n", body)
	assert.Equal(t, `attachment; filename="report.csv"`, resp.Header.Get("Content-Disposition"))
}

// An explicit filename overrides the one on disk, which is what lets a
// generated temp file arrive as something the user can recognise.
func TestResponseDownloadWithExplicitName(t *testing.T) {
	path := tempFile(t, "tmp-98f2c1.csv", "id,name\n1,Ada\n")

	resp, _ := serveResponse(t, func(r *Response) error {
		return r.Download(path, "sales-2024.csv")
	})

	assert.Equal(t, `attachment; filename="sales-2024.csv"`, resp.Header.Get("Content-Disposition"))
}

// --- RedirectRoute ---

// RedirectRoute resolves the name against the router that dispatched the
// request, so a route named "users.index" registered at "/users" sends the
// browser to "/users". Parameters that match no placeholder are dropped,
// as they are by ctx.RedirectToRoute.
func TestResponseRedirectRouteResolvesNamedRoutes(t *testing.T) {
	app := newTestApp()
	router := NewRouter(&mockApplication{}, app)

	router.GET("/users", func(ctx *Context) error {
		return ctx.String("the users index")
	}).Name("users.index")

	router.GET("/go", func(ctx *Context) error {
		return NewResponse(ctx.FiberCtx()).RedirectRoute("users.index", map[string]any{"page": 2})
	})

	resp, err := app.Test(httptest.NewRequest("GET", "/go", nil), -1)
	require.NoError(t, err)

	assert.Equal(t, fiber.StatusFound, resp.StatusCode)
	assert.Equal(t, "/users", resp.Header.Get("Location"))
}

// Parameters are substituted into the route's placeholders, and escaped.
func TestResponseRedirectRouteSubstitutesParams(t *testing.T) {
	app := newTestApp()
	router := NewRouter(&mockApplication{}, app)

	router.GET("/users/:id/posts/:slug", func(ctx *Context) error {
		return ctx.String("a post")
	}).Name("users.posts.show")

	router.GET("/go", func(ctx *Context) error {
		return NewResponse(ctx.FiberCtx()).RedirectRoute("users.posts.show", map[string]any{
			"id":   7,
			"slug": "hello world",
		})
	})

	resp, err := app.Test(httptest.NewRequest("GET", "/go", nil), -1)
	require.NoError(t, err)

	assert.Equal(t, fiber.StatusFound, resp.StatusCode)
	assert.Equal(t, "/users/7/posts/hello%20world", resp.Header.Get("Location"))
}

// An unknown name is an error rather than a redirect to a path built out
// of the name: that path would 404, and the caller could not tell the
// mistake from a route that really is missing.
func TestResponseRedirectRouteUnknownNameErrors(t *testing.T) {
	app := newTestApp()
	router := NewRouter(&mockApplication{}, app)

	var redirectErr error
	router.GET("/go", func(ctx *Context) error {
		redirectErr = NewResponse(ctx.FiberCtx()).RedirectRoute("users.missing")
		return ctx.String("still here")
	})

	resp, err := app.Test(httptest.NewRequest("GET", "/go", nil), -1)
	require.NoError(t, err)

	require.Error(t, redirectErr)
	assert.Contains(t, redirectErr.Error(), `route "users.missing" is not defined`)
	assert.Empty(t, resp.Header.Get("Location"), "nothing was sent")
}

// A Response built outside the router - raw Fiber middleware, say - has no
// route table to consult, and says so rather than guessing.
func TestResponseRedirectRouteWithoutRouterErrors(t *testing.T) {
	app := newTestApp()

	var redirectErr error
	app.Get("/go", func(c *fiber.Ctx) error {
		redirectErr = NewResponse(c).RedirectRoute("users.index")
		return c.SendString("still here")
	})

	_, err := app.Test(httptest.NewRequest("GET", "/go", nil), -1)
	require.NoError(t, err)

	require.Error(t, redirectErr)
	assert.Contains(t, redirectErr.Error(), "no router available")
}

// --- Stream ---

func TestResponseStream(t *testing.T) {
	resp, body := serveResponse(t, func(r *Response) error {
		return r.Stream("text/csv", strings.NewReader("id,name\n1,Ada\n"))
	})

	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, "id,name\n1,Ada\n", body)
	assert.Contains(t, resp.Header.Get("Content-Type"), "text/csv")
}

func TestResponseStreamEmptyReader(t *testing.T) {
	resp, body := serveResponse(t, func(r *Response) error {
		return r.Stream("application/octet-stream", strings.NewReader(""))
	})

	assert.Equal(t, 200, resp.StatusCode)
	assert.Empty(t, body)
}

// --- Flush ---

// Flush is a no-op: Fiber buffers and writes the whole response itself.
func TestResponseFlushIsANoOp(t *testing.T) {
	var flushErr error
	resp, body := serveResponse(t, func(r *Response) error {
		if err := r.String("partial"); err != nil {
			return err
		}
		flushErr = r.Flush()
		return nil
	})

	assert.NoError(t, flushErr)
	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, "partial", body)
}

// --- Type / Vary / Append ---

func TestResponseType(t *testing.T) {
	cases := []struct {
		given string
		want  string
	}{
		{"json", "application/json"},
		{"html", "text/html"},
		{"csv", "text/csv"},
	}

	for _, tc := range cases {
		t.Run(tc.given, func(t *testing.T) {
			resp, _ := serveResponse(t, func(r *Response) error {
				assert.Same(t, r, r.Type(tc.given), "Type is chainable")
				return r.String("body")
			})

			assert.Contains(t, resp.Header.Get("Content-Type"), tc.want)
		})
	}
}

func TestResponseVary(t *testing.T) {
	resp, _ := serveResponse(t, func(r *Response) error {
		assert.Same(t, r, r.Vary("Accept-Encoding", "Origin"))
		r.Vary("Accept-Language")
		return r.String("body")
	})

	assert.Equal(t, "Accept-Encoding, Origin, Accept-Language", resp.Header.Get("Vary"))
}

// Vary does not repeat a header it already lists.
func TestResponseVaryIsIdempotent(t *testing.T) {
	resp, _ := serveResponse(t, func(r *Response) error {
		r.Vary("Origin")
		r.Vary("Origin")
		return r.String("body")
	})

	assert.Equal(t, "Origin", resp.Header.Get("Vary"))
}

func TestResponseVaryWithNoHeaders(t *testing.T) {
	resp, _ := serveResponse(t, func(r *Response) error {
		r.Vary()
		return r.String("body")
	})

	assert.Empty(t, resp.Header.Get("Vary"))
}

// Append adds a value without discarding the ones already there, which
// is what a header allowed to repeat needs.
func TestResponseAppend(t *testing.T) {
	resp, _ := serveResponse(t, func(r *Response) error {
		assert.Same(t, r, r.Append("X-Trace", "first"))
		r.Append("X-Trace", "second")
		return r.String("body")
	})

	// Fiber folds repeated appends into one comma-separated value rather
	// than emitting the header twice.
	assert.Equal(t, "first, second", resp.Header.Get("X-Trace"))
	assert.Len(t, resp.Header.Values("X-Trace"), 1)
}

// --- Cookie builder ---

// NewCookie starts from the defaults a session cookie wants: rooted at
// "/", hidden from scripts and same-site by default.
func TestNewCookieDefaults(t *testing.T) {
	cookie := NewCookie("session", "abc123")

	assert.Equal(t, "session", cookie.Name)
	assert.Equal(t, "abc123", cookie.Value)
	assert.Equal(t, "/", cookie.Path)
	assert.True(t, cookie.HTTPOnly)
	assert.Equal(t, "Lax", cookie.SameSite)
	assert.Empty(t, cookie.Domain)
	assert.Zero(t, cookie.MaxAge)
	assert.False(t, cookie.Secure)
}

// Every With* setter mutates and returns the same cookie, so they chain.
func TestCookieBuilderSetters(t *testing.T) {
	cookie := NewCookie("session", "abc123")

	returned := cookie.
		WithPath("/admin").
		WithDomain("example.com").
		WithMaxAge(3600).
		WithSecure(true).
		WithHTTPOnly(false).
		WithSameSite("Strict")

	assert.Same(t, cookie, returned, "the builder mutates in place")
	assert.Equal(t, "/admin", cookie.Path)
	assert.Equal(t, "example.com", cookie.Domain)
	assert.Equal(t, 3600, cookie.MaxAge)
	assert.True(t, cookie.Secure)
	assert.False(t, cookie.HTTPOnly)
	assert.Equal(t, "Strict", cookie.SameSite)
}

// A negative max age is how a cookie is expired, and the builder passes
// it through untouched.
func TestCookieBuilderNegativeMaxAge(t *testing.T) {
	cookie := NewCookie("session", "").WithMaxAge(-1)
	assert.Equal(t, -1, cookie.MaxAge)
}

// The attributes the builder holds reach the wire: NewCookie returns the
// very type Response.Cookie accepts, so the builder is handed straight to
// the setter with no field-by-field copy in between.
func TestCookieBuilderAttributesReachTheWire(t *testing.T) {
	built := NewCookie("session", "abc123").
		WithPath("/admin").
		WithDomain("example.com").
		WithMaxAge(3600).
		WithSecure(true).
		WithSameSite("Strict")

	// The builder's result is a *contracts.Cookie; http.Cookie is an alias.
	var _ *contracts.Cookie = built

	resp, _ := serveResponse(t, func(r *Response) error {
		r.Cookie(built)
		return r.String("ok")
	})

	setCookie := resp.Header.Get("Set-Cookie")
	assert.Contains(t, setCookie, "session=abc123")
	assert.Contains(t, setCookie, "path=/admin")
	assert.Contains(t, setCookie, "domain=example.com")
	assert.Contains(t, setCookie, "max-age=3600")
	assert.Contains(t, setCookie, "secure")
	assert.Contains(t, setCookie, "HttpOnly")
	assert.Contains(t, setCookie, "SameSite=Strict")

	parsed := (&http.Response{Header: resp.Header}).Cookies()
	require.Len(t, parsed, 1)
	assert.Equal(t, "session", parsed[0].Name)
	assert.Equal(t, "abc123", parsed[0].Value)
	assert.Equal(t, "/admin", parsed[0].Path)
	assert.Equal(t, 3600, parsed[0].MaxAge)
	assert.True(t, parsed[0].Secure)
	assert.True(t, parsed[0].HttpOnly)
}
