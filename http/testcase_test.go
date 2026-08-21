package http_test

import (
	"fmt"
	"testing"

	genhttp "github.com/genesysflow/go-genesys/http"
	"github.com/stretchr/testify/assert"
)

// recordingT captures assertion failures instead of failing the test,
// so failure paths can themselves be tested.
type recordingT struct{ failures []string }

func (r *recordingT) Helper() {}
func (r *recordingT) Errorf(format string, args ...any) {
	r.failures = append(r.failures, fmt.Sprintf(format, args...))
}

func demoKernel(t *testing.T) *genhttp.Kernel {
	t.Helper()
	k := bootKernel(t)
	k.GET("/users", func(ctx *genhttp.Context) error {
		return ctx.JSONResponse(map[string]any{
			"data": []map[string]any{
				{"name": "Ada", "admin": true},
				{"name": "Bob", "admin": false},
			},
			"total": 2,
		})
	})
	k.POST("/users", func(ctx *genhttp.Context) error {
		var body map[string]any
		ctx.JSON(&body)
		return ctx.Created(map[string]any{"name": body["name"]})
	})
	k.GET("/page", func(ctx *genhttp.Context) error {
		ctx.Header("X-Page", "home")
		return ctx.HTML("<h1>Welcome home</h1>")
	})
	k.GET("/away", func(ctx *genhttp.Context) error { return ctx.Redirect("/page") })
	k.GET("/auth", func(ctx *genhttp.Context) error {
		return ctx.String(ctx.Request().Header("Authorization"))
	})
	return k
}

func TestTestCaseHappyPath(t *testing.T) {
	tc := genhttp.NewTestCase(t, demoKernel(t))

	tc.Get("/users").
		AssertOK().
		AssertJson(map[string]any{"total": 2}).
		AssertJsonPath("data.0.name", "Ada").
		AssertJsonPath("data.1.admin", false).
		AssertJsonCount("data", 2)

	tc.Post("/users", map[string]any{"name": "Carol"}).
		AssertCreated().
		AssertJsonPath("name", "Carol")

	tc.Get("/page").
		AssertOK().
		AssertHeader("X-Page", "home").
		AssertSee("Welcome home").
		AssertDontSee("error")

	tc.Get("/away").AssertRedirect("/page")
	tc.Get("/missing").AssertNotFound()
}

func TestTestCaseDefaultHeaders(t *testing.T) {
	tc := genhttp.NewTestCase(t, demoKernel(t)).WithToken("secret-token")
	tc.Get("/auth").AssertSee("Bearer secret-token")
}

func TestAssertionsReportFailures(t *testing.T) {
	rec := &recordingT{}
	tc := genhttp.NewTestCase(rec, demoKernel(t))

	tc.Get("/users").AssertStatus(500)
	tc.Get("/users").AssertJsonPath("data.0.name", "Nobody")
	tc.Get("/users").AssertJsonPath("data.9.name", "OutOfRange")
	tc.Get("/users").AssertJsonCount("data", 7)
	tc.Get("/page").AssertDontSee("Welcome")
	tc.Get("/page").AssertRedirect()

	assert.Len(t, rec.failures, 6)
	assert.Contains(t, rec.failures[0], "expected status 500")
	assert.Contains(t, rec.failures[2], "bad array index")
}
