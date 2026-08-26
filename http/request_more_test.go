package http

import (
	"bytes"
	"encoding/base64"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// serveRequest drives httpReq through a handler that receives the
// framework's Request wrapper, and returns the response.
func serveRequest(t *testing.T, method, route string, httpReq *http.Request, handler func(*Request) error) *http.Response {
	t.Helper()

	app := newTestApp()
	app.Add(method, route, func(c *fiber.Ctx) error {
		return handler(NewRequest(c))
	})

	resp, err := app.Test(httpReq, -1)
	require.NoError(t, err)
	return resp
}

// --- URL and client address ---

func TestRequestFullURL(t *testing.T) {
	httpReq := httptest.NewRequest("GET", "/search?q=go&page=2", nil)
	httpReq.Host = "example.com"

	serveRequest(t, "GET", "/search", httpReq, func(r *Request) error {
		assert.Equal(t, "http://example.com/search?q=go&page=2", r.FullURL())
		return r.ctx.SendString("ok")
	})
}

// With no query string the full URL is just scheme, host and path.
func TestRequestFullURLWithoutQuery(t *testing.T) {
	httpReq := httptest.NewRequest("GET", "/about", nil)
	httpReq.Host = "docs.example.com"

	serveRequest(t, "GET", "/about", httpReq, func(r *Request) error {
		assert.Equal(t, "http://docs.example.com/about", r.FullURL())
		return r.ctx.SendString("ok")
	})
}

// X-Forwarded-For is attacker-controlled, so with no trusted proxies
// configured IPs() ignores it and reports the connecting address, exactly
// as IP() does. A caller that rate-limits or allowlists on IPs()[0] can
// therefore not be fed an address by the client.
func TestRequestIPsIgnoresForwardedForFromUntrustedClient(t *testing.T) {
	httpReq := httptest.NewRequest("GET", "/ip", nil)
	httpReq.Header.Set("X-Forwarded-For", "203.0.113.1, 198.51.100.7,192.0.2.9")

	serveRequest(t, "GET", "/ip", httpReq, func(r *Request) error {
		assert.Equal(t, []string{r.IP()}, r.IPs())
		assert.NotContains(t, r.IPs(), "203.0.113.1", "the header is not believed")
		return r.ctx.SendString("ok")
	})
}

// Once the proxy the request arrived from is trusted, the forwarded chain
// is honoured, left to right. The config here is what the kernel builds
// out of KernelConfig.TrustedProxies.
func TestRequestIPsReadsForwardedForFromTrustedProxy(t *testing.T) {
	app := fiber.New(fiber.Config{
		DisableStartupMessage:   true,
		EnableTrustedProxyCheck: true,
		// The address Fiber's test connection reports.
		TrustedProxies: []string{"0.0.0.0"},
		ProxyHeader:    fiber.HeaderXForwardedFor,
	})

	var ips []string
	app.Get("/ip", func(c *fiber.Ctx) error {
		ips = NewRequest(c).IPs()
		return c.SendString("ok")
	})

	httpReq := httptest.NewRequest("GET", "/ip", nil)
	httpReq.Header.Set("X-Forwarded-For", "203.0.113.1, 198.51.100.7,192.0.2.9")

	_, err := app.Test(httpReq, -1)
	require.NoError(t, err)

	assert.Equal(t, []string{"203.0.113.1", "198.51.100.7", "192.0.2.9"}, ips)
}

// Without the header there is still a client: the address the connection
// came from, the one IP() reports.
func TestRequestIPsWithoutHeader(t *testing.T) {
	serveRequest(t, "GET", "/ip", httptest.NewRequest("GET", "/ip", nil), func(r *Request) error {
		assert.Equal(t, []string{r.IP()}, r.IPs())
		return r.ctx.SendString("ok")
	})
}

// --- Only / Except ---

// Only and Except draw on the same input All() does: query string, form
// body and route parameters together.
func TestRequestOnlyAndExcept(t *testing.T) {
	httpReq := httptest.NewRequest("POST", "/items/42?q=search&page=2", strings.NewReader("name=Ada&role=admin"))
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	serveRequest(t, "POST", "/items/:id", httpReq, func(r *Request) error {
		assert.Equal(t, map[string]any{
			"name": "Ada",
			"q":    "search",
		}, r.Only("name", "q", "absent"), "Only skips keys that were never sent")

		except := r.Except("name", "q")
		assert.Equal(t, "2", except["page"])
		assert.Equal(t, "admin", except["role"])
		assert.Equal(t, "42", except["id"], "route parameters are input too")
		assert.NotContains(t, except, "name")
		assert.NotContains(t, except, "q")

		return r.ctx.SendString("ok")
	})
}

// Only with no keys returns an empty map, Except with no keys returns
// everything.
func TestRequestOnlyAndExceptDegenerateCases(t *testing.T) {
	httpReq := httptest.NewRequest("GET", "/x?a=1&b=2", nil)

	serveRequest(t, "GET", "/x", httpReq, func(r *Request) error {
		assert.Empty(t, r.Only())
		assert.Equal(t, map[string]any{"a": "1", "b": "2"}, r.Except())
		return r.ctx.SendString("ok")
	})
}

// --- Has / Filled ---

// Has asks whether a key was sent, Filled whether it carries a value. The
// pair is what lets a handler tell "the user cleared this field" from "the
// client never sent it", which for a PATCH decides between blanking a
// column and leaving it alone.
func TestRequestHasAndFilled(t *testing.T) {
	httpReq := httptest.NewRequest("GET", "/f?present=value&empty=&spaces=%20%20", nil)

	serveRequest(t, "GET", "/f", httpReq, func(r *Request) error {
		assert.True(t, r.Has("present"))
		assert.True(t, r.Filled("present"))

		assert.True(t, r.Has("empty"), "sent, though empty")
		assert.False(t, r.Filled("empty"), "and so not filled")

		assert.True(t, r.Has("spaces"), "Has does not trim")
		assert.False(t, r.Filled("spaces"), "Filled trims before deciding")

		assert.False(t, r.Has("absent"))
		assert.False(t, r.Filled("absent"))

		return r.ctx.SendString("ok")
	})
}

// A key sent in the form body is present too, empty value and all.
func TestRequestHasReadsFormBody(t *testing.T) {
	httpReq := httptest.NewRequest("POST", "/f", strings.NewReader("name=Ada&nickname="))
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	serveRequest(t, "POST", "/f", httpReq, func(r *Request) error {
		assert.True(t, r.Has("name"))
		assert.True(t, r.Has("nickname"), "the user cleared it; it was still sent")
		assert.False(t, r.Filled("nickname"))
		assert.False(t, r.Has("absent"))
		return r.ctx.SendString("ok")
	})
}

// Route parameters are input, as they are for Input and All.
func TestRequestHasReadsRouteParams(t *testing.T) {
	serveRequest(t, "GET", "/items/:id", httptest.NewRequest("GET", "/items/42", nil), func(r *Request) error {
		assert.True(t, r.Has("id"))
		assert.True(t, r.Filled("id"))
		return r.ctx.SendString("ok")
	})
}

// Has also sees a JSON body, which for an API request is all the input
// there is.
func TestRequestHasReadsJSONBody(t *testing.T) {
	httpReq := httptest.NewRequest("POST", "/f", strings.NewReader(`{"title":"Hello","draft":false,"blank":""}`))
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")

	serveRequest(t, "POST", "/f", httpReq, func(r *Request) error {
		assert.True(t, r.Has("title"))
		assert.True(t, r.Filled("title"))
		assert.True(t, r.Has("draft"), "false stringifies to \"false\"")
		assert.True(t, r.Has("blank"), "sent as an empty string, so present")
		assert.False(t, r.Filled("blank"))
		return r.ctx.SendString("ok")
	})
}

// --- Body ---

func TestRequestBodyAndBodyReader(t *testing.T) {
	payload := `{"title":"Hello, world"}`
	httpReq := httptest.NewRequest("POST", "/b", strings.NewReader(payload))
	httpReq.Header.Set("Content-Type", "application/json")

	serveRequest(t, "POST", "/b", httpReq, func(r *Request) error {
		assert.Equal(t, []byte(payload), r.Body())

		read, err := io.ReadAll(r.BodyReader())
		require.NoError(t, err)
		assert.Equal(t, payload, string(read))

		// The reader is independent of the body, so reading it does not
		// consume what a later BodyParser would need.
		assert.Equal(t, []byte(payload), r.Body())

		return r.ctx.SendString("ok")
	})
}

func TestRequestBodyEmpty(t *testing.T) {
	serveRequest(t, "GET", "/b", httptest.NewRequest("GET", "/b", nil), func(r *Request) error {
		assert.Empty(t, r.Body())

		read, err := io.ReadAll(r.BodyReader())
		require.NoError(t, err)
		assert.Empty(t, read)

		return r.ctx.SendString("ok")
	})
}

// --- File uploads ---

// multipartRequest builds an upload carrying the given files, keyed by
// form field, plus a plain text field.
func multipartRequest(t *testing.T, path string, files map[string][]string) *http.Request {
	t.Helper()

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	require.NoError(t, writer.WriteField("caption", "holiday"))

	for field, names := range files {
		for _, name := range names {
			part, err := writer.CreateFormFile(field, name)
			require.NoError(t, err)
			_, err = part.Write([]byte("contents of " + name))
			require.NoError(t, err)
		}
	}
	require.NoError(t, writer.Close())

	req := httptest.NewRequest("POST", path, &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req
}

func TestRequestFile(t *testing.T) {
	httpReq := multipartRequest(t, "/upload", map[string][]string{
		"avatar": {"portrait.png"},
	})

	serveRequest(t, "POST", "/upload", httpReq, func(r *Request) error {
		header, err := r.File("avatar")
		require.NoError(t, err)
		require.NotNil(t, header)
		assert.Equal(t, "portrait.png", header.Filename)
		assert.Equal(t, int64(len("contents of portrait.png")), header.Size)

		opened, err := header.Open()
		require.NoError(t, err)
		defer opened.Close()
		contents, err := io.ReadAll(opened)
		require.NoError(t, err)
		assert.Equal(t, "contents of portrait.png", string(contents))

		return r.ctx.SendString("ok")
	})
}

func TestRequestFileMissingKey(t *testing.T) {
	httpReq := multipartRequest(t, "/upload", map[string][]string{
		"avatar": {"portrait.png"},
	})

	serveRequest(t, "POST", "/upload", httpReq, func(r *Request) error {
		header, err := r.File("banner")
		assert.Error(t, err)
		assert.Nil(t, header)
		return r.ctx.SendString("ok")
	})
}

func TestRequestFiles(t *testing.T) {
	httpReq := multipartRequest(t, "/upload", map[string][]string{
		"docs":   {"a.txt", "b.txt"},
		"avatar": {"portrait.png"},
	})

	serveRequest(t, "POST", "/upload", httpReq, func(r *Request) error {
		docs, err := r.Files("docs")
		require.NoError(t, err)
		require.Len(t, docs, 2)
		assert.Equal(t, "a.txt", docs[0].Filename)
		assert.Equal(t, "b.txt", docs[1].Filename)

		avatars, err := r.Files("avatar")
		require.NoError(t, err)
		assert.Len(t, avatars, 1)

		absent, err := r.Files("banner")
		require.NoError(t, err, "an unknown key is not an error, just no files")
		assert.Empty(t, absent)

		// The text field travelled with them.
		assert.Equal(t, "holiday", r.Input("caption"))

		return r.ctx.SendString("ok")
	})
}

// A request that is not multipart reports the parse failure rather than
// pretending it carried no files.
func TestRequestFilesOnNonMultipartRequest(t *testing.T) {
	httpReq := httptest.NewRequest("POST", "/upload", strings.NewReader(`{"a":1}`))
	httpReq.Header.Set("Content-Type", "application/json")

	serveRequest(t, "POST", "/upload", httpReq, func(r *Request) error {
		files, err := r.Files("docs")
		assert.Error(t, err)
		assert.Nil(t, files)
		return r.ctx.SendString("ok")
	})
}

// --- Cookies ---

func TestRequestCookieAndCookies(t *testing.T) {
	httpReq := httptest.NewRequest("GET", "/dash", nil)
	httpReq.AddCookie(&http.Cookie{Name: "session", Value: "abc123"})
	httpReq.AddCookie(&http.Cookie{Name: "theme", Value: "dark"})

	serveRequest(t, "GET", "/dash", httpReq, func(r *Request) error {
		assert.Equal(t, "abc123", r.Cookie("session"))
		assert.Equal(t, "dark", r.Cookie("theme"))
		assert.Empty(t, r.Cookie("absent"))

		assert.Equal(t, map[string]string{
			"session": "abc123",
			"theme":   "dark",
		}, r.Cookies())

		return r.ctx.SendString("ok")
	})
}

func TestRequestCookiesEmpty(t *testing.T) {
	serveRequest(t, "GET", "/dash", httptest.NewRequest("GET", "/dash", nil), func(r *Request) error {
		assert.Empty(t, r.Cookies())
		assert.Empty(t, r.Cookie("session"))
		return r.ctx.SendString("ok")
	})
}

// --- Per-request store ---

func TestRequestGetSet(t *testing.T) {
	serveRequest(t, "GET", "/s", httptest.NewRequest("GET", "/s", nil), func(r *Request) error {
		assert.Nil(t, r.Get("missing"))

		r.Set("tenant", "acme")
		r.Set("attempts", 3)
		assert.Equal(t, "acme", r.Get("tenant"))
		assert.Equal(t, 3, r.Get("attempts"))

		r.Set("tenant", "globex")
		assert.Equal(t, "globex", r.Get("tenant"), "a later Set replaces the value")

		return r.ctx.SendString("ok")
	})
}

// --- Header conveniences ---

func TestRequestContentType(t *testing.T) {
	cases := []struct {
		header string
		want   string
	}{
		{"application/json", "application/json"},
		{"application/json; charset=utf-8", "application/json; charset=utf-8"},
		{"", ""},
	}

	for _, tc := range cases {
		httpReq := httptest.NewRequest("POST", "/ct", strings.NewReader("x"))
		httpReq.Header.Del("Content-Type")
		if tc.header != "" {
			httpReq.Header.Set("Content-Type", tc.header)
		}

		serveRequest(t, "POST", "/ct", httpReq, func(r *Request) error {
			assert.Equal(t, tc.want, r.ContentType())
			return r.ctx.SendString("ok")
		})
	}
}

func TestRequestUserAgentAndReferer(t *testing.T) {
	httpReq := httptest.NewRequest("GET", "/p", nil)
	httpReq.Header.Set("User-Agent", "genesys-tests/1.0")
	httpReq.Header.Set("Referer", "https://example.com/previous")

	serveRequest(t, "GET", "/p", httpReq, func(r *Request) error {
		assert.Equal(t, "genesys-tests/1.0", r.UserAgent())
		assert.Equal(t, "https://example.com/previous", r.Referer())
		return r.ctx.SendString("ok")
	})
}

func TestRequestUserAgentAndRefererAbsent(t *testing.T) {
	httpReq := httptest.NewRequest("GET", "/p", nil)
	httpReq.Header.Del("User-Agent")

	serveRequest(t, "GET", "/p", httpReq, func(r *Request) error {
		assert.Empty(t, r.UserAgent())
		assert.Empty(t, r.Referer())
		return r.ctx.SendString("ok")
	})
}

// --- Authorization ---

func TestRequestBearerToken(t *testing.T) {
	cases := []struct {
		name   string
		header string
		want   string
	}{
		{"standard", "Bearer abc.def.ghi", "abc.def.ghi"},
		{"lowercase scheme", "bearer abc.def.ghi", "abc.def.ghi"},
		{"mixed case scheme", "BeArEr abc.def.ghi", "abc.def.ghi"},
		{"extra whitespace", "Bearer   abc.def.ghi  ", "abc.def.ghi"},
		{"no scheme", "abc.def.ghi", ""},
		{"wrong scheme", "Basic YWRhOnMzY3JldA==", ""},
		{"scheme only", "Bearer", ""},
		{"empty token", "Bearer ", ""},
		{"absent", "", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			httpReq := httptest.NewRequest("GET", "/me", nil)
			if tc.header != "" {
				httpReq.Header.Set("Authorization", tc.header)
			}

			serveRequest(t, "GET", "/me", httpReq, func(r *Request) error {
				assert.Equal(t, tc.want, r.BearerToken())
				return r.ctx.SendString("ok")
			})
		})
	}
}

func TestRequestBasicAuth(t *testing.T) {
	encode := func(s string) string {
		return base64.StdEncoding.EncodeToString([]byte(s))
	}

	cases := []struct {
		name         string
		header       string
		wantUser     string
		wantPassword string
		wantOK       bool
	}{
		{"valid", "Basic " + encode("ada:s3cret"), "ada", "s3cret", true},
		{"lowercase scheme", "basic " + encode("ada:s3cret"), "ada", "s3cret", true},
		{"password contains colons", "Basic " + encode("ada:pa:ss:word"), "ada", "pa:ss:word", true},
		{"empty password", "Basic " + encode("ada:"), "ada", "", true},
		{"empty username", "Basic " + encode(":s3cret"), "", "s3cret", true},
		{"no colon in credentials", "Basic " + encode("adas3cret"), "", "", false},
		{"not base64", "Basic ada:s3cret", "", "", false},
		{"wrong scheme", "Bearer " + encode("ada:s3cret"), "", "", false},
		{"scheme only", "Basic", "", "", false},
		{"absent", "", "", "", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			httpReq := httptest.NewRequest("GET", "/me", nil)
			if tc.header != "" {
				httpReq.Header.Set("Authorization", tc.header)
			}

			serveRequest(t, "GET", "/me", httpReq, func(r *Request) error {
				user, password, ok := r.BasicAuth()
				assert.Equal(t, tc.wantOK, ok)
				assert.Equal(t, tc.wantUser, user)
				assert.Equal(t, tc.wantPassword, password)
				return r.ctx.SendString("ok")
			})
		})
	}
}
