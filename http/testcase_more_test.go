package http_test

import (
	"testing"

	"github.com/genesysflow/go-genesys/contracts"
	genhttp "github.com/genesysflow/go-genesys/http"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// statusKernel answers each path with the status its name gives, so the
// status assertions can be exercised without a real application.
func statusKernel(t *testing.T) *genhttp.Kernel {
	t.Helper()

	k := bootKernel(t)

	k.DELETE("/posts/1", func(ctx *genhttp.Context) error { return ctx.NoContent() })
	k.POST("/bad", func(ctx *genhttp.Context) error {
		return ctx.Status(400).JSONResponse(map[string]any{"error": "malformed cursor"})
	})
	k.GET("/private", func(ctx *genhttp.Context) error {
		return ctx.Status(401).JSONResponse(map[string]any{"error": "unauthenticated"})
	})
	k.GET("/admin", func(ctx *genhttp.Context) error {
		return ctx.Status(403).JSONResponse(map[string]any{"error": "forbidden"})
	})
	k.POST("/users", func(ctx *genhttp.Context) error {
		return ctx.Status(422).JSONResponse(map[string]any{
			"message": "The given data was invalid.",
			"errors": map[string]any{
				"email": []string{"The email field is required."},
			},
		})
	})

	return k
}

// Each status assertion passes on its own code.
func TestTestCaseStatusAssertions(t *testing.T) {
	tc := genhttp.NewTestCase(t, statusKernel(t))

	tc.Delete("/posts/1").AssertNoContent()
	tc.Post("/bad").AssertBadRequest()
	tc.Get("/private").AssertUnauthorized()
	tc.Get("/admin").AssertForbidden()
	tc.Post("/users", map[string]any{}).AssertUnprocessable()
}

// And fails, loudly, on any other code.
func TestTestCaseStatusAssertionsFailOnMismatch(t *testing.T) {
	k := bootKernel(t)
	k.GET("/ok", func(ctx *genhttp.Context) error { return ctx.String("ok") })
	k.POST("/ok", func(ctx *genhttp.Context) error { return ctx.String("ok") })
	k.DELETE("/ok", func(ctx *genhttp.Context) error { return ctx.String("ok") })

	spy := &recordingT{}
	tc := genhttp.NewTestCase(spy, k)

	tc.Delete("/ok").AssertNoContent()
	tc.Post("/ok").AssertBadRequest()
	tc.Get("/ok").AssertUnauthorized()
	tc.Get("/ok").AssertForbidden()
	tc.Post("/ok").AssertUnprocessable()

	require.Len(t, spy.failures, 5)
	assert.Contains(t, spy.failures[0], "expected status 204")
	assert.Contains(t, spy.failures[1], "expected status 400")
	assert.Contains(t, spy.failures[2], "expected status 401")
	assert.Contains(t, spy.failures[3], "expected status 403")
	assert.Contains(t, spy.failures[4], "expected status 422")
}

// AssertValidationError checks both halves of Laravel's 422 payload: the
// status and an entry for the field.
func TestTestCaseAssertValidationError(t *testing.T) {
	genhttp.NewTestCase(t, statusKernel(t)).
		Post("/users", map[string]any{}).
		AssertValidationError("email")
}

// It fails when the field is not among the errors, and when the response
// was never a 422 at all.
func TestTestCaseAssertValidationErrorFails(t *testing.T) {
	k := statusKernel(t)
	k.GET("/ok", func(ctx *genhttp.Context) error { return ctx.JSONResponse(map[string]any{"ok": true}) })

	spy := &recordingT{}
	tc := genhttp.NewTestCase(spy, k)

	tc.Post("/users", map[string]any{}).AssertValidationError("name")
	assert.Len(t, spy.failures, 1)
	assert.Contains(t, spy.failures[0], `validation error for "name"`)

	spy.failures = nil
	tc.Get("/ok").AssertValidationError("email")
	assert.Len(t, spy.failures, 2, "the wrong status and the missing field both report")
}

// --- headers, cookies and redirects ---

func cookieKernel(t *testing.T) *genhttp.Kernel {
	t.Helper()

	k := bootKernel(t)

	k.GET("/set", func(ctx *genhttp.Context) error {
		ctx.Cookie(&contracts.Cookie{Name: "session", Value: "abc123", Path: "/"})
		ctx.Cookie(&contracts.Cookie{Name: "theme", Value: "dark", Path: "/"})
		return ctx.String("set")
	})
	k.GET("/plain", func(ctx *genhttp.Context) error { return ctx.String("plain") })
	k.GET("/echo-cookie", func(ctx *genhttp.Context) error {
		return ctx.String(ctx.Request().Cookie("session"))
	})
	k.POST("/posts", func(ctx *genhttp.Context) error {
		return ctx.Redirect("/posts/98f2c1/edit")
	})

	return k
}

// AssertCookie reads the cookie the response set.
func TestTestCaseAssertCookie(t *testing.T) {
	genhttp.NewTestCase(t, cookieKernel(t)).
		Get("/set").
		AssertOK().
		AssertCookie("session", "abc123").
		AssertCookie("theme", "dark")
}

// It fails both when the cookie is absent and when its value differs.
func TestTestCaseAssertCookieFails(t *testing.T) {
	spy := &recordingT{}
	tc := genhttp.NewTestCase(spy, cookieKernel(t))

	tc.Get("/set").AssertCookie("session", "wrong")
	tc.Get("/plain").AssertCookie("session", "abc123")

	require.Len(t, spy.failures, 2)
	assert.Contains(t, spy.failures[0], `expected cookie session to be "wrong"`)
	assert.Contains(t, spy.failures[1], "expected the response to set the cookie session")
}

// The jar is readable, so a test can carry a value out of a response.
func TestTestCaseCookieAccessor(t *testing.T) {
	tc := genhttp.NewTestCase(t, cookieKernel(t))

	assert.Empty(t, tc.Cookie("session"), "the jar starts empty")

	tc.Get("/set").AssertOK()
	assert.Equal(t, "abc123", tc.Cookie("session"))
	assert.Equal(t, "dark", tc.Cookie("theme"))
	assert.Empty(t, tc.Cookie("absent"))

	tc.FlushCookies()
	assert.Empty(t, tc.Cookie("session"))
}

// A cookie set explicitly on a request wins over the jar's, which is how
// a test overrides one value without discarding the whole session.
func TestTestCaseExplicitCookieOverridesTheJar(t *testing.T) {
	tc := genhttp.NewTestCase(t, cookieKernel(t))

	tc.Get("/set").AssertOK()
	require.Equal(t, "abc123", tc.Cookie("session"))

	tc.Get("/echo-cookie").AssertSee("abc123")
	tc.Do(genhttp.Get("/echo-cookie").WithCookie("session", "explicit")).
		AssertOK().
		AssertSee("explicit")

	assert.Equal(t, "abc123", tc.Cookie("session"), "the jar itself is unchanged")
}

// AssertHeaderMissing guards against a header leaking back.
func TestTestCaseAssertHeaderMissing(t *testing.T) {
	k := bootKernel(t)
	k.GET("/plain", func(ctx *genhttp.Context) error {
		ctx.Header("X-Debug-Query", "select * from users")
		return ctx.String("plain")
	})
	k.GET("/clean", func(ctx *genhttp.Context) error { return ctx.String("clean") })

	genhttp.NewTestCase(t, k).Get("/clean").AssertHeaderMissing("X-Debug-Query")

	spy := &recordingT{}
	genhttp.NewTestCase(spy, k).Get("/plain").AssertHeaderMissing("X-Debug-Query")

	require.Len(t, spy.failures, 1)
	assert.Contains(t, spy.failures[0], "expected no X-Debug-Query header")
}

// AssertLocationContains is for a redirect whose target carries a
// generated id, where the whole URL is not known in advance.
func TestTestCaseAssertLocationContains(t *testing.T) {
	tc := genhttp.NewTestCase(t, cookieKernel(t))

	tc.Post("/posts").
		AssertRedirect().
		AssertLocationContains("/posts/").
		AssertLocationContains("/edit")
}

func TestTestCaseAssertLocationContainsFails(t *testing.T) {
	spy := &recordingT{}
	tc := genhttp.NewTestCase(spy, cookieKernel(t))

	tc.Post("/posts").AssertLocationContains("/comments/")
	tc.Get("/plain").AssertLocationContains("/anything")

	require.Len(t, spy.failures, 2)
	assert.Contains(t, spy.failures[0], "expected the redirect target to contain")
	assert.Contains(t, spy.failures[1], `got ""`, "a response with no Location reports an empty target")
}

// The assertions that need a *testing.T are inert without one, so a
// TestResponse obtained outside a test case does not panic.
func TestAssertionsAreInertWithoutATestingT(t *testing.T) {
	resp, err := cookieKernel(t).Test(genhttp.Get("/plain"))
	require.NoError(t, err)

	assert.NotPanics(t, func() {
		resp.AssertCookie("session", "abc123")
		resp.AssertHeaderMissing("Content-Type")
		resp.AssertLocationContains("/anywhere")
		resp.AssertJsonMissing("data")
	})
}

// --- request bodies on the write verbs ---

// Put and Patch marshal an optional body the same way Post does.
func TestTestCaseWriteVerbsSendJSONBodies(t *testing.T) {
	k := bootKernel(t)
	echo := func(ctx *genhttp.Context) error {
		var body map[string]any
		_ = ctx.JSON(&body)
		return ctx.JSONResponse(map[string]any{
			"method":       ctx.Method(),
			"title":        body["title"],
			"content_type": ctx.Request().Header("Content-Type"),
		})
	}
	k.PUT("/posts/1", echo)
	k.PATCH("/posts/1", echo)

	tc := genhttp.NewTestCase(t, k)

	tc.Put("/posts/1", map[string]any{"title": "Replaced"}).
		AssertOK().
		AssertJsonPath("method", "PUT").
		AssertJsonPath("title", "Replaced").
		AssertJsonPath("content_type", "application/json")

	tc.Patch("/posts/1", map[string]any{"title": "Amended"}).
		AssertOK().
		AssertJsonPath("method", "PATCH").
		AssertJsonPath("title", "Amended")
}

// Only the first body is sent, matching the other variadic helpers.
func TestTestCaseWriteVerbsUseFirstBodyOnly(t *testing.T) {
	k := bootKernel(t)
	k.PUT("/posts/1", func(ctx *genhttp.Context) error {
		var body map[string]any
		_ = ctx.JSON(&body)
		return ctx.JSONResponse(body)
	})

	genhttp.NewTestCase(t, k).
		Put("/posts/1", map[string]any{"title": "First"}, map[string]any{"title": "Second"}).
		AssertJsonPath("title", "First")
}

// --- assertion failure paths ---

// Every content assertion reports rather than passing quietly.
func TestTestCaseContentAssertionsFailLoudly(t *testing.T) {
	k := bootKernel(t)
	k.GET("/page", func(ctx *genhttp.Context) error {
		ctx.Header("X-Page", "home")
		return ctx.HTML("<h1>Welcome home</h1>")
	})
	k.GET("/users", func(ctx *genhttp.Context) error {
		return ctx.JSONResponse(map[string]any{
			"total": 2,
			"data":  []map[string]any{{"name": "Ada"}, {"name": "Bob"}},
		})
	})

	spy := &recordingT{}
	tc := genhttp.NewTestCase(spy, k)

	tc.Get("/page").AssertHeader("X-Page", "away")
	tc.Get("/page").AssertSee("Goodbye")
	tc.Get("/page").AssertJson(map[string]any{"total": 2})
	tc.Get("/users").AssertJson(map[string]any{"missing": 1})
	tc.Get("/users").AssertJson(map[string]any{"total": 99})
	tc.Get("/users").AssertJsonCount("total", 2)
	tc.Get("/users").AssertJsonCount("absent", 2)

	require.Len(t, spy.failures, 7)
	assert.Contains(t, spy.failures[0], `expected header X-Page="away"`)
	assert.Contains(t, spy.failures[1], "expected body to contain")
	assert.Contains(t, spy.failures[2], "response is not JSON")
	assert.Contains(t, spy.failures[3], `expected JSON key "missing"`)
	assert.Contains(t, spy.failures[4], `expected JSON "total" to be 99`)
	assert.Contains(t, spy.failures[5], "to be an array")
	assert.Contains(t, spy.failures[6], "not found")
}

// --- the cookie jar follows the server ---

// A cookie the server clears leaves the jar, so the next request does
// not keep sending a session the server has already discarded.
func TestTestCaseJarDropsClearedCookies(t *testing.T) {
	k := bootKernel(t)
	k.GET("/login", func(ctx *genhttp.Context) error {
		ctx.Cookie(&contracts.Cookie{Name: "session", Value: "abc123", Path: "/"})
		return ctx.String("in")
	})
	k.GET("/logout", func(ctx *genhttp.Context) error {
		ctx.ClearCookie("session")
		return ctx.String("out")
	})
	k.GET("/whoami", func(ctx *genhttp.Context) error {
		value := ctx.Request().Cookie("session")
		if value == "" {
			value = "guest"
		}
		return ctx.String(value)
	})

	tc := genhttp.NewTestCase(t, k)

	tc.Get("/login").AssertOK()
	require.Equal(t, "abc123", tc.Cookie("session"))
	tc.Get("/whoami").AssertSee("abc123")

	tc.Get("/logout").AssertOK()
	assert.Empty(t, tc.Cookie("session"), "a cleared cookie leaves the jar")
	tc.Get("/whoami").AssertSee("guest")
}
