package middleware_test

import (
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/genesysflow/go-genesys/foundation"
	genhttp "github.com/genesysflow/go-genesys/http"
	"github.com/genesysflow/go-genesys/http/middleware"
	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// overrideKernel answers with the verb that was dispatched, so a test
// can tell which route the request actually reached.
func overrideKernel(t *testing.T, config ...middleware.MethodOverrideConfig) *genhttp.Kernel {
	t.Helper()

	app := foundation.New()
	require.NoError(t, app.Boot())

	kernel := genhttp.NewKernel(app, genhttp.KernelConfig{
		DisableStartupMessage: true,
		PreRouting:            []fiber.Handler{middleware.MethodOverride(config...)},
	})

	echo := func(verb string) genhttp.HandlerFunc {
		return func(ctx *genhttp.Context) error { return ctx.String(verb) }
	}
	kernel.GET("/thing", echo("GET"))
	kernel.POST("/thing", echo("POST"))
	kernel.PUT("/thing", echo("PUT"))
	kernel.PATCH("/thing", echo("PATCH"))
	kernel.DELETE("/thing", echo("DELETE"))

	return kernel
}

// form sends a urlencoded body, the way a browser submits a form.
func form(t *testing.T, kernel *genhttp.Kernel, method, target, body string) (int, string) {
	t.Helper()

	request := httptest.NewRequest(method, target, strings.NewReader(body))
	request.Header.Set("Content-Type", fiber.MIMEApplicationForm)

	return send(t, kernel, request)
}

func send(t *testing.T, kernel *genhttp.Kernel, request *http.Request) (int, string) {
	t.Helper()

	response, err := kernel.Fiber().Test(request, -1)
	require.NoError(t, err)

	content := make([]byte, 64)
	n, _ := response.Body.Read(content)
	return response.StatusCode, string(content[:n])
}

// The whole point: a form that can only POST reaches the verb it named.
func TestMethodOverrideReachesTheDeclaredRoute(t *testing.T) {
	kernel := overrideKernel(t)

	for _, verb := range []string{"PUT", "PATCH", "DELETE"} {
		status, body := form(t, kernel, "POST", "/thing", "_method="+verb)
		assert.Equal(t, 200, status, verb)
		assert.Equal(t, verb, body, "a POST declaring %s should reach the %s route", verb, verb)
	}
}

// The value is normalised, because a form is written by hand and a hand
// writes "delete".
func TestMethodOverrideIsCaseInsensitiveAndTrimmed(t *testing.T) {
	kernel := overrideKernel(t)

	status, body := form(t, kernel, "POST", "/thing", "_method=+delete+")
	assert.Equal(t, 200, status)
	assert.Equal(t, "DELETE", body)
}

// Anything outside the allowlist leaves the request as the POST it was,
// rather than being refused: a stray field is not an attack, and failing
// the request would break a form that happens to use the name.
func TestMethodOverrideIgnoresAVerbAFormMayNotDeclare(t *testing.T) {
	kernel := overrideKernel(t)

	for _, declared := range []string{"GET", "HEAD", "OPTIONS", "TRACE", "CONNECT", "FOO", ""} {
		status, body := form(t, kernel, "POST", "/thing", "_method="+declared)
		assert.Equal(t, 200, status, declared)
		assert.Equal(t, "POST", body, "a POST declaring %q should stay a POST", declared)
	}
}

// A GET is never rewritten. This is the rule that keeps a link, a
// prefetch or a crawler from deleting something.
func TestMethodOverrideNeverRewritesAGet(t *testing.T) {
	kernel := overrideKernel(t)

	status, body := send(t, kernel, httptest.NewRequest("GET", "/thing?_method=DELETE", nil))
	assert.Equal(t, 200, status)
	assert.Equal(t, "GET", body)
}

// The query string is not a source. A verb there could be smuggled into
// a URL that a redirect, a referrer or a log will carry around.
func TestMethodOverrideIgnoresTheQueryString(t *testing.T) {
	kernel := overrideKernel(t)

	status, body := form(t, kernel, "POST", "/thing?_method=DELETE", "")
	assert.Equal(t, 200, status)
	assert.Equal(t, "POST", body)

	// Nor does a query value override the body's own answer.
	status, body = form(t, kernel, "POST", "/thing?_method=DELETE", "_method=PUT")
	assert.Equal(t, 200, status)
	assert.Equal(t, "PUT", body)
}

// A client behind a proxy that refuses the verb outright declares it in
// a header instead.
func TestMethodOverrideHonoursTheHeader(t *testing.T) {
	kernel := overrideKernel(t)

	request := httptest.NewRequest("POST", "/thing", nil)
	request.Header.Set(middleware.DefaultMethodHeader, "delete")

	status, body := send(t, kernel, request)
	assert.Equal(t, 200, status)
	assert.Equal(t, "DELETE", body)

	// The same allowlist applies to it.
	request = httptest.NewRequest("POST", "/thing", nil)
	request.Header.Set(middleware.DefaultMethodHeader, "GET")

	status, body = send(t, kernel, request)
	assert.Equal(t, 200, status)
	assert.Equal(t, "POST", body)
}

// The body wins over the header: a form's own field is the more
// deliberate statement of the two.
func TestMethodOverridePrefersTheBodyOverTheHeader(t *testing.T) {
	kernel := overrideKernel(t)

	request := httptest.NewRequest("POST", "/thing", strings.NewReader("_method=PUT"))
	request.Header.Set("Content-Type", fiber.MIMEApplicationForm)
	request.Header.Set(middleware.DefaultMethodHeader, "DELETE")

	status, body := send(t, kernel, request)
	assert.Equal(t, 200, status)
	assert.Equal(t, "PUT", body)
}

// An operator who wants the header surface gone can have it gone.
func TestMethodOverrideHeaderCanBeDisabled(t *testing.T) {
	kernel := overrideKernel(t, middleware.MethodOverrideConfig{DisableHeader: true})

	request := httptest.NewRequest("POST", "/thing", nil)
	request.Header.Set(middleware.DefaultMethodHeader, "DELETE")

	status, body := send(t, kernel, request)
	assert.Equal(t, 200, status)
	assert.Equal(t, "POST", body)

	// The field still works.
	status, body = form(t, kernel, "POST", "/thing", "_method=DELETE")
	assert.Equal(t, 200, status)
	assert.Equal(t, "DELETE", body)
}

// An upload form is multipart, and its fields are not where a urlencoded
// body keeps them.
func TestMethodOverrideReadsAMultipartField(t *testing.T) {
	kernel := overrideKernel(t)

	var body strings.Builder
	writer := multipart.NewWriter(&body)
	require.NoError(t, writer.WriteField("_method", "PUT"))
	require.NoError(t, writer.Close())

	request := httptest.NewRequest("POST", "/thing", strings.NewReader(body.String()))
	request.Header.Set("Content-Type", writer.FormDataContentType())

	status, answered := send(t, kernel, request)
	assert.Equal(t, 200, status)
	assert.Equal(t, "PUT", answered)
}

// The field name is configurable, for an application with its own
// convention.
func TestMethodOverrideHonoursACustomFieldName(t *testing.T) {
	kernel := overrideKernel(t, middleware.MethodOverrideConfig{FieldName: "__verb"})

	status, body := form(t, kernel, "POST", "/thing", "__verb=DELETE")
	assert.Equal(t, 200, status)
	assert.Equal(t, "DELETE", body)

	status, body = form(t, kernel, "POST", "/thing", "_method=DELETE")
	assert.Equal(t, 200, status)
	assert.Equal(t, "POST", body)
}

// Without the middleware the browser's form does not work at all, which
// is the bug this exists to fix - and the reason it cannot be an
// http.MiddlewareFunc: by the time one runs, the router has answered.
func TestWithoutMethodOverrideAFormCannotReachTheVerb(t *testing.T) {
	app := foundation.New()
	require.NoError(t, app.Boot())

	kernel := genhttp.NewKernel(app, genhttp.KernelConfig{DisableStartupMessage: true})
	kernel.DELETE("/thing", func(ctx *genhttp.Context) error { return ctx.String("DELETE") })

	status, _ := form(t, kernel, "POST", "/thing", "_method=DELETE")
	assert.Equal(t, 405, status)
}
