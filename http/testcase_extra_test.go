package http_test

import (
	"net/url"
	"testing"

	"github.com/genesysflow/go-genesys/foundation"
	genhttp "github.com/genesysflow/go-genesys/http"
	"github.com/genesysflow/go-genesys/providers"
	"github.com/genesysflow/go-genesys/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type actor struct {
	ID    int64
	Email string
}

func testKernel(t *testing.T) *genhttp.Kernel {
	t.Helper()

	app := foundation.New()
	require.NoError(t, app.Register(&providers.AppServiceProvider{}))
	require.NoError(t, app.Register(&providers.ValidationServiceProvider{}))
	require.NoError(t, app.Boot())

	k := genhttp.NewKernel(app, genhttp.KernelConfig{DisableStartupMessage: true})
	k.UseFiber(session.NewManager(session.Config{CookieSecure: false}).Middleware())
	return k
}

// ActingAs authenticates the requests a test makes, so a guarded route
// can be exercised without logging in through the real flow.
func TestActingAs(t *testing.T) {
	k := testKernel(t)
	k.GET("/me", func(ctx *genhttp.Context) error {
		user, ok := genhttp.UserAs[actor](ctx)
		if !ok {
			return ctx.Status(401).String("guest")
		}
		return ctx.String(user.Email)
	})

	tc := genhttp.NewTestCase(t, k)

	tc.Get("/me").AssertStatus(401).AssertSee("guest")

	tc.ActingAs(&actor{ID: 1, Email: "ada@example.com"}).
		Get("/me").
		AssertOK().
		AssertSee("ada@example.com")
}

// ActingAsGuest undoes it, so one test case can cover both sides.
func TestActingAsGuest(t *testing.T) {
	k := testKernel(t)
	k.GET("/me", func(ctx *genhttp.Context) error {
		if ctx.HasUser() {
			return ctx.String("user")
		}
		return ctx.String("guest")
	})

	tc := genhttp.NewTestCase(t, k).ActingAs(&actor{ID: 1})
	tc.Get("/me").AssertSee("user")

	tc.ActingAsGuest().Get("/me").AssertSee("guest")
}

// Cookies carry between requests, so a session survives a redirect the
// way a browser's would.
func TestCookiesPersistBetweenRequests(t *testing.T) {
	k := testKernel(t)
	k.POST("/flash", func(ctx *genhttp.Context) error {
		require.NoError(t, ctx.Session().Flash("status", "Saved!"))
		return ctx.Back("/form").Send()
	})
	k.GET("/form", func(ctx *genhttp.Context) error {
		value, _ := ctx.Session().Get("status").(string)
		return ctx.String(value)
	})

	tc := genhttp.NewTestCase(t, k)
	tc.Post("/flash").AssertRedirect("/form")
	tc.Get("/form").AssertOK().AssertSee("Saved!")
}

// A test that wants isolation can drop the jar.
func TestFlushCookies(t *testing.T) {
	k := testKernel(t)
	k.GET("/set", func(ctx *genhttp.Context) error {
		require.NoError(t, ctx.Session().Set("marker", "kept"))
		return ctx.String("set")
	})
	k.GET("/read", func(ctx *genhttp.Context) error {
		value, _ := ctx.Session().Get("marker").(string)
		if value == "" {
			value = "empty"
		}
		return ctx.String(value)
	})

	tc := genhttp.NewTestCase(t, k)
	tc.Get("/set")
	tc.Get("/read").AssertSee("kept")

	tc.FlushCookies()
	tc.Get("/read").AssertSee("empty")
}

// Validation errors flashed for a browser are assertable without
// digging through the session by hand.
type storeRequest struct {
	Name string `json:"name" form:"name" validate:"required,min=3"`
}

func TestAssertSessionHasErrors(t *testing.T) {
	k := testKernel(t)
	k.POST("/users", func(ctx *genhttp.Context) error {
		_, err := genhttp.ValidateRequest[storeRequest](ctx)
		if err != nil {
			return err
		}
		return ctx.String("ok")
	})
	k.GET("/users/create", func(ctx *genhttp.Context) error {
		return ctx.JSONResponse(ctx.Errors().Messages())
	})

	tc := genhttp.NewTestCase(t, k).WithHeader("Accept", "text/html").WithHeader("Referer", "/users/create")

	form := url.Values{}
	form.Set("name", "Al")
	tc.PostForm("/users", map[string]string{"name": "Al"}).AssertRedirect()

	tc.Get("/users/create").AssertOK().AssertSee("name")
}

// AssertJsonMissing guards against a field leaking back into a response.
func TestAssertJsonMissing(t *testing.T) {
	k := testKernel(t)
	k.GET("/me", func(ctx *genhttp.Context) error {
		return ctx.JSONResponse(map[string]any{"email": "ada@example.com"})
	})

	genhttp.NewTestCase(t, k).Get("/me").
		AssertOK().
		AssertJsonPath("email", "ada@example.com").
		AssertJsonMissing("password")
}

// A failed assertion must report, not pass quietly.
func TestAssertionsFailLoudly(t *testing.T) {
	k := testKernel(t)
	k.GET("/me", func(ctx *genhttp.Context) error {
		return ctx.JSONResponse(map[string]any{"password": "leaked"})
	})

	spy := &recordingT{}
	genhttp.NewTestCase(spy, k).Get("/me").AssertJsonMissing("password")

	assert.NotEmpty(t, spy.failures, "a leaked field should fail the assertion")
}
