package http

import (
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// capturingT records assertion failures instead of failing the test, so
// the toolkit's own failure paths can be exercised. The external
// http_test package has its own copy; this one exists because
// TestResponse's fields are unexported.
type capturingT struct{ failures []string }

func (c *capturingT) Helper() {}
func (c *capturingT) Errorf(format string, args ...any) {
	c.failures = append(c.failures, fmt.Sprintf(format, args...))
}

// --- TestRequest builders ---

// Every verb helper produces a request the standard library agrees with.
func TestTestRequestVerbHelpers(t *testing.T) {
	cases := []struct {
		name   string
		build  func(string) *TestRequest
		method string
	}{
		{"Get", Get, "GET"},
		{"Post", Post, "POST"},
		{"Put", Put, "PUT"},
		{"Patch", Patch, "PATCH"},
		{"Delete", Delete, "DELETE"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			built := tc.build("/widgets/7")
			assert.Equal(t, tc.method, built.method)
			assert.Equal(t, "/widgets/7", built.path)

			req := built.toHTTPRequest()
			assert.Equal(t, tc.method, req.Method)
			assert.Equal(t, "/widgets/7", req.URL.Path)
			assert.Zero(t, req.ContentLength, "a request with no body must not carry one")
		})
	}
}

// A path carrying a query string survives into the parsed URL.
func TestTestRequestKeepsQueryString(t *testing.T) {
	req := Get("/search?q=go&page=2").toHTTPRequest()

	assert.Equal(t, "/search", req.URL.Path)
	assert.Equal(t, "go", req.URL.Query().Get("q"))
	assert.Equal(t, "2", req.URL.Query().Get("page"))
}

// WithHeader and WithHeaders both land on the outgoing request, and the
// later call wins for a key set twice.
func TestTestRequestWithHeaders(t *testing.T) {
	built := Get("/h").
		WithHeader("X-One", "1").
		WithHeader("X-Overwritten", "before").
		WithHeaders(map[string]string{
			"X-Two":         "2",
			"X-Three":       "3",
			"X-Overwritten": "after",
		})

	assert.Equal(t, "after", built.headers["X-Overwritten"])

	req := built.toHTTPRequest()
	assert.Equal(t, "1", req.Header.Get("X-One"))
	assert.Equal(t, "2", req.Header.Get("X-Two"))
	assert.Equal(t, "3", req.Header.Get("X-Three"))
	assert.Equal(t, "after", req.Header.Get("X-Overwritten"))
}

// WithHeaders on an empty map is a no-op rather than a nil map panic.
func TestTestRequestWithHeadersEmpty(t *testing.T) {
	built := Get("/h").WithHeaders(map[string]string{})
	assert.Empty(t, built.headers)
	assert.NotNil(t, built.toHTTPRequest())
}

// WithBody sends the bytes verbatim and sets no content type: a caller
// passing raw bytes is the one who knows what they are.
func TestTestRequestWithBody(t *testing.T) {
	req := Post("/raw").WithBody([]byte("hello raw body")).toHTTPRequest()

	require.NotNil(t, req.Body)
	body, err := io.ReadAll(req.Body)
	require.NoError(t, err)
	assert.Equal(t, "hello raw body", string(body))
	assert.Empty(t, req.Header.Get("Content-Type"))
}

// An empty body is treated as no body at all.
func TestTestRequestWithEmptyBody(t *testing.T) {
	req := Post("/raw").WithBody([]byte{}).toHTTPRequest()

	assert.Zero(t, req.ContentLength)
	body, err := io.ReadAll(req.Body)
	require.NoError(t, err)
	assert.Empty(t, body)
}

// WithJSON marshals and declares the content type, which is what makes
// the framework's Input() read the body.
func TestTestRequestWithJSONSetsContentType(t *testing.T) {
	req := Post("/api").WithJSON(map[string]string{"name": "Ada"}).toHTTPRequest()

	assert.Equal(t, "application/json", req.Header.Get("Content-Type"))
	body, err := io.ReadAll(req.Body)
	require.NoError(t, err)
	assert.JSONEq(t, `{"name":"Ada"}`, string(body))
}

// A value that cannot be marshalled leaves the request untouched rather
// than panicking.
func TestTestRequestWithJSONUnmarshalable(t *testing.T) {
	built := Post("/api").WithJSON(make(chan int))

	assert.Nil(t, built.body)
	assert.Empty(t, built.headers["Content-Type"])
}

// WithBearerToken writes the RFC 7235 scheme the request side parses.
func TestTestRequestWithBearerToken(t *testing.T) {
	req := Get("/me").WithBearerToken("abc.def.ghi").toHTTPRequest()

	assert.Equal(t, "Bearer abc.def.ghi", req.Header.Get("Authorization"))
}

// WithBasicAuth writes RFC 7617 credentials: the user-pass pair base64
// encoded, which is what any standards-compliant parser expects to read
// back - here, net/http's own.
func TestTestRequestWithBasicAuthIsBase64Encoded(t *testing.T) {
	req := Get("/me").WithBasicAuth("ada", "s3cret").toHTTPRequest()

	encoded := "Basic " + base64.StdEncoding.EncodeToString([]byte("ada:s3cret"))
	assert.Equal(t, encoded, req.Header.Get("Authorization"))

	user, password, ok := req.BasicAuth()
	require.True(t, ok, "net/http decodes the header WithBasicAuth writes")
	assert.Equal(t, "ada", user)
	assert.Equal(t, "s3cret", password)
}

// The framework's own request-side parser reads them back too, so a test
// can build credentials with the toolkit and exercise the handler that
// consumes them.
func TestTestRequestBasicAuthRoundTrips(t *testing.T) {
	built := Get("/me").WithBasicAuth("ada", "s3cret")

	app := newTestApp()
	var gotOK bool
	var gotUser, gotPassword string
	app.Get("/me", func(c *fiber.Ctx) error {
		gotUser, gotPassword, gotOK = NewRequest(c).BasicAuth()
		return c.SendString("done")
	})

	resp, err := app.Test(built.toHTTPRequest(), -1)
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode)

	assert.True(t, gotOK)
	assert.Equal(t, "ada", gotUser)
	assert.Equal(t, "s3cret", gotPassword)
}

// A colon is legal in a password and survives the round trip: only the
// first one separates the pair.
func TestTestRequestBasicAuthWithColonInPassword(t *testing.T) {
	req := Get("/me").WithBasicAuth("ada", "pa:ss:word").toHTTPRequest()

	user, password, ok := req.BasicAuth()
	require.True(t, ok)
	assert.Equal(t, "ada", user)
	assert.Equal(t, "pa:ss:word", password)
}

// WithCookie is additive and every cookie reaches the request.
func TestTestRequestWithCookies(t *testing.T) {
	req := Get("/dash").
		WithCookie("session", "abc123").
		WithCookie("theme", "dark").
		toHTTPRequest()

	session, err := req.Cookie("session")
	require.NoError(t, err)
	assert.Equal(t, "abc123", session.Value)

	theme, err := req.Cookie("theme")
	require.NoError(t, err)
	assert.Equal(t, "dark", theme.Value)
}

// hasCookie is what stops a test case's jar overriding an explicit cookie.
func TestTestRequestHasCookie(t *testing.T) {
	built := Get("/dash").WithCookie("session", "abc123")

	assert.True(t, built.hasCookie("session"))
	assert.False(t, built.hasCookie("theme"))
}

// The builders compose: every part of a fully specified request arrives.
func TestTestRequestBuildersCompose(t *testing.T) {
	req := Put("/posts/1?draft=true").
		WithJSON(map[string]any{"title": "Hello"}).
		WithHeader("X-Request-Id", "req-1").
		WithBearerToken("tok").
		WithCookie("session", "s").
		toHTTPRequest()

	assert.Equal(t, "PUT", req.Method)
	assert.Equal(t, "/posts/1", req.URL.Path)
	assert.Equal(t, "true", req.URL.Query().Get("draft"))
	assert.Equal(t, "application/json", req.Header.Get("Content-Type"))
	assert.Equal(t, "req-1", req.Header.Get("X-Request-Id"))
	assert.Equal(t, "Bearer tok", req.Header.Get("Authorization"))

	cookie, err := req.Cookie("session")
	require.NoError(t, err)
	assert.Equal(t, "s", cookie.Value)
}

// --- TestResponse ---

// recordedResponse builds a TestResponse the way the toolkit does, from
// a finished *http.Response.
func recordedResponse(t *testing.T, code int, body string, headers map[string][]string) *TestResponse {
	t.Helper()

	rec := httptest.NewRecorder()
	for key, values := range headers {
		for _, value := range values {
			rec.Header().Add(key, value)
		}
	}
	rec.WriteHeader(code)
	if body != "" {
		_, err := rec.WriteString(body)
		require.NoError(t, err)
	}

	return newTestResponse(rec.Result())
}

// Status, Body and BodyString read the recorded response.
func TestTestResponseAccessors(t *testing.T) {
	resp := recordedResponse(t, 201, `{"id":7}`, map[string][]string{
		"Content-Type": {"application/json"},
		"X-Trace":      {"trace-1"},
	})

	assert.Equal(t, 201, resp.Status())
	assert.Equal(t, []byte(`{"id":7}`), resp.Body())
	assert.Equal(t, `{"id":7}`, resp.BodyString())
	assert.Equal(t, "trace-1", resp.Header("X-Trace"))
	assert.Empty(t, resp.Header("X-Absent"))
}

// Headers exposes the whole header set, including one sent twice.
func TestTestResponseHeaders(t *testing.T) {
	resp := recordedResponse(t, 200, "", map[string][]string{
		"X-Multi":      {"first", "second"},
		"Content-Type": {"text/plain"},
	})

	headers := resp.Headers()
	assert.IsType(t, http.Header{}, headers)
	assert.Equal(t, []string{"first", "second"}, headers.Values("X-Multi"))
	assert.Equal(t, "text/plain", headers.Get("Content-Type"))
	assert.Equal(t, "first", headers.Get("X-Multi"), "Get returns the first value")
}

// JSON decodes the body into the caller's value.
func TestTestResponseJSONDecodes(t *testing.T) {
	resp := recordedResponse(t, 200, `{"name":"Ada","admin":true,"tags":["a","b"]}`, nil)

	var decoded struct {
		Name  string   `json:"name"`
		Admin bool     `json:"admin"`
		Tags  []string `json:"tags"`
	}
	require.NoError(t, resp.JSON(&decoded))
	assert.Equal(t, "Ada", decoded.Name)
	assert.True(t, decoded.Admin)
	assert.Equal(t, []string{"a", "b"}, decoded.Tags)
}

// A body that is not JSON reports the decode error rather than leaving
// the caller with a zero value it cannot distinguish from an empty one.
func TestTestResponseJSONReportsError(t *testing.T) {
	resp := recordedResponse(t, 500, "<html>oops</html>", nil)

	var decoded map[string]any
	err := resp.JSON(&decoded)
	assert.Error(t, err)
	assert.Nil(t, decoded)
}

// An empty body is an error too, not silent success.
func TestTestResponseJSONOnEmptyBody(t *testing.T) {
	resp := recordedResponse(t, 204, "", nil)

	var decoded map[string]any
	assert.Error(t, resp.JSON(&decoded))
}

// Cookies returns everything the response set; Cookie picks one out.
func TestTestResponseCookies(t *testing.T) {
	resp := recordedResponse(t, 200, "", map[string][]string{
		"Set-Cookie": {
			"session=abc123; Path=/; HttpOnly",
			"theme=dark; Path=/; Max-Age=600",
		},
	})

	cookies := resp.Cookies()
	require.Len(t, cookies, 2)

	names := []string{cookies[0].Name, cookies[1].Name}
	sort.Strings(names)
	assert.Equal(t, []string{"session", "theme"}, names)

	session := resp.Cookie("session")
	require.NotNil(t, session)
	assert.Equal(t, "abc123", session.Value)
	assert.Equal(t, "/", session.Path)
	assert.True(t, session.HttpOnly)

	theme := resp.Cookie("theme")
	require.NotNil(t, theme)
	assert.Equal(t, 600, theme.MaxAge)

	assert.Nil(t, resp.Cookie("absent"), "a cookie that was not set reports nil")
}

// A response that set no cookies reports an empty slice and nil lookups.
func TestTestResponseNoCookies(t *testing.T) {
	resp := recordedResponse(t, 200, "ok", nil)

	assert.Empty(t, resp.Cookies())
	assert.Nil(t, resp.Cookie("session"))
}

// Every Is* predicate, across the boundaries of each class of status.
// IsServerError is deliberately open-ended (>= 500) while the others are
// closed ranges, so 599 and above are all server errors.
func TestTestResponseStatusPredicates(t *testing.T) {
	all := []string{
		"IsOK", "IsCreated", "IsNoContent", "IsBadRequest", "IsUnauthorized",
		"IsForbidden", "IsNotFound", "IsInternalServerError", "IsRedirect",
		"IsSuccess", "IsClientError", "IsServerError",
	}

	evaluate := func(r *TestResponse) map[string]bool {
		return map[string]bool{
			"IsOK":                  r.IsOK(),
			"IsCreated":             r.IsCreated(),
			"IsNoContent":           r.IsNoContent(),
			"IsBadRequest":          r.IsBadRequest(),
			"IsUnauthorized":        r.IsUnauthorized(),
			"IsForbidden":           r.IsForbidden(),
			"IsNotFound":            r.IsNotFound(),
			"IsInternalServerError": r.IsInternalServerError(),
			"IsRedirect":            r.IsRedirect(),
			"IsSuccess":             r.IsSuccess(),
			"IsClientError":         r.IsClientError(),
			"IsServerError":         r.IsServerError(),
		}
	}

	cases := []struct {
		code int
		want []string
	}{
		{199, nil},
		{200, []string{"IsOK", "IsSuccess"}},
		{201, []string{"IsCreated", "IsSuccess"}},
		{204, []string{"IsNoContent", "IsSuccess"}},
		{299, []string{"IsSuccess"}},
		{300, []string{"IsRedirect"}},
		{302, []string{"IsRedirect"}},
		{399, []string{"IsRedirect"}},
		{400, []string{"IsBadRequest", "IsClientError"}},
		{401, []string{"IsUnauthorized", "IsClientError"}},
		{403, []string{"IsForbidden", "IsClientError"}},
		{404, []string{"IsNotFound", "IsClientError"}},
		{499, []string{"IsClientError"}},
		{500, []string{"IsInternalServerError", "IsServerError"}},
		{503, []string{"IsServerError"}},
		{599, []string{"IsServerError"}},
	}

	for _, tc := range cases {
		t.Run(fmt.Sprint(tc.code), func(t *testing.T) {
			got := evaluate(recordedResponse(t, tc.code, "", nil))

			expected := make(map[string]bool, len(all))
			for _, name := range tc.want {
				expected[name] = true
			}

			for _, name := range all {
				assert.Equal(t, expected[name], got[name], "%s at status %d", name, tc.code)
			}
		})
	}
}

// A request that never produced a response must fail loudly rather than
// pass an assertion by accident.
func TestTestResponseAssertionsOnAbsentResponse(t *testing.T) {
	spy := &capturingT{}
	resp := &TestResponse{t: spy}

	resp.AssertStatus(200)
	resp.AssertRedirect()

	require.Len(t, spy.failures, 2)
	assert.Contains(t, spy.failures[0], "no response")
	assert.Contains(t, spy.failures[1], "no response")
}
