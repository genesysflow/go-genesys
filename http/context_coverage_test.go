package http

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/genesysflow/go-genesys/container"
	"github.com/genesysflow/go-genesys/validation"
	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Validate ---

type validatedPayload struct {
	Email string `validate:"required,email"`
	Age   int    `validate:"gte=18"`
}

// validatingApplication is a mockApplication that can resolve a real
// validator, so Context.Validate can be exercised without booting the
// whole framework.
type validatingApplication struct {
	mockApplication
	validator *validation.Validator
}

func (v *validatingApplication) Make(key string) (any, error) {
	if v.validator != nil && key == container.GetTypeName(reflect.TypeOf(v.validator)) {
		return v.validator, nil
	}
	return nil, fmt.Errorf("container: service %q is not bound", key)
}

func TestContextValidatePasses(t *testing.T) {
	app := &validatingApplication{validator: validation.New()}

	fiberApp := newTestApp()
	router := NewRouter(app, fiberApp)
	router.GET("/", func(ctx *Context) error {
		assert.NoError(t, ctx.Validate(&validatedPayload{Email: "ada@example.com", Age: 36}))
		return ctx.String("ok")
	})

	resp, err := fiberApp.Test(httptest.NewRequest("GET", "/", nil), -1)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestContextValidateFails(t *testing.T) {
	app := &validatingApplication{validator: validation.New()}

	fiberApp := newTestApp()
	router := NewRouter(app, fiberApp)

	var returned error
	router.GET("/", func(ctx *Context) error {
		returned = ctx.Validate(&validatedPayload{Email: "not-an-email", Age: 12})
		return ctx.String("ok")
	})

	resp, err := fiberApp.Test(httptest.NewRequest("GET", "/", nil), -1)
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode)

	require.Error(t, returned)
	failures, ok := returned.(*validation.ValidationErrors)
	require.True(t, ok, "expected validation errors, got %T", returned)
	assert.False(t, failures.IsEmpty())
	assert.True(t, failures.Has("Email"), "all: %v", failures.All())
	assert.True(t, failures.Has("Age"))
}

// Without a validator bound, Validate reports the resolution failure
// rather than silently accepting the input.
func TestContextValidateWithoutValidatorBound(t *testing.T) {
	var returned error
	runHandler(t, func(ctx *Context) error {
		returned = ctx.Validate(&validatedPayload{})
		return ctx.String("ok")
	})

	assert.Error(t, returned)
}

// --- SetNext / Next ---

func TestContextSetNextAndNext(t *testing.T) {
	runHandler(t, func(ctx *Context) error {
		calls := 0
		ctx.SetNext(func() error {
			calls++
			return nil
		})

		require.NoError(t, ctx.Next())
		require.NoError(t, ctx.Next())
		assert.Equal(t, 2, calls)

		return ctx.String("ok")
	})
}

// An error from the stored next reaches the caller.
func TestContextNextPropagatesError(t *testing.T) {
	runHandler(t, func(ctx *Context) error {
		ctx.SetNext(func() error { return fmt.Errorf("downstream failed") })

		err := ctx.Next()
		require.Error(t, err)
		assert.Equal(t, "downstream failed", err.Error())

		return ctx.String("ok")
	})
}

// With nothing set, Next falls through to Fiber's own chain.
func TestContextNextFallsThroughToFiber(t *testing.T) {
	app := newTestApp()
	app.Get("/chain",
		func(c *fiber.Ctx) error {
			return NewContext(c, &mockApplication{}).Next()
		},
		func(c *fiber.Ctx) error {
			return c.SendString("second handler")
		},
	)

	resp, err := app.Test(httptest.NewRequest("GET", "/chain", nil), -1)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

// The router publishes each middleware's continuation on the context as
// well as passing it as the `next` argument, so ctx.Next() and next() are
// the same call. The two spellings look interchangeable and are: were
// ctx.next left nil, ctx.Next() would drop into Fiber's chain, skipping
// every remaining MiddlewareFunc and the route handler.
func TestRouterMiddlewarePopulatesContextNext(t *testing.T) {
	app := newTestApp()
	router := NewRouter(&mockApplication{}, app)

	var order []string
	router.Use(func(ctx *Context, next func() error) error {
		order = append(order, "first")
		return ctx.Next()
	})
	router.Use(func(ctx *Context, next func() error) error {
		order = append(order, "second")
		return next()
	})
	router.GET("/", func(ctx *Context) error {
		order = append(order, "handler")
		return ctx.String("ok")
	})

	resp, err := app.Test(httptest.NewRequest("GET", "/", nil), -1)
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, "ok", string(body))
	assert.Equal(t, []string{"first", "second", "handler"}, order)
}

// A middleware that does not continue the chain still stops it, whichever
// spelling the ones before it used.
func TestRouterMiddlewareContextNextStopsWhenNotCalled(t *testing.T) {
	app := newTestApp()
	router := NewRouter(&mockApplication{}, app)

	handlerRan := false
	router.Use(func(ctx *Context, next func() error) error {
		return ctx.Next()
	})
	router.Use(func(ctx *Context, next func() error) error {
		return ctx.String("stopped here")
	})
	router.GET("/", func(ctx *Context) error {
		handlerRan = true
		return ctx.String("ok")
	})

	resp, err := app.Test(httptest.NewRequest("GET", "/", nil), -1)
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, "stopped here", string(body))
	assert.False(t, handlerRan)
}

// The handler ends the chain, so ctx.Next() inside it must not re-enter
// the handler: the router clears the continuation before calling it.
func TestRouterHandlerNextDoesNotRecurse(t *testing.T) {
	app := newTestApp()
	router := NewRouter(&mockApplication{}, app)

	calls := 0
	router.Use(func(ctx *Context, next func() error) error { return next() })
	router.GET("/", func(ctx *Context) error {
		calls++
		_ = ctx.Next()
		return ctx.String("ok")
	})

	_, err := app.Test(httptest.NewRequest("GET", "/", nil), -1)
	require.NoError(t, err)
	assert.Equal(t, 1, calls)
}

// --- AbortWithError ---

func TestContextAbortWithError(t *testing.T) {
	resp, body := runHandler(t, func(ctx *Context) error {
		err := ctx.AbortWithError(503, fmt.Errorf("upstream unavailable"))
		assert.True(t, ctx.IsAborted())
		return err
	})

	assert.Equal(t, 503, resp.StatusCode)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal([]byte(body), &decoded))
	assert.Equal(t, "upstream unavailable", decoded["error"])
}

// Aborting from middleware stops the chain before the handler runs.
func TestContextAbortWithErrorStopsTheChain(t *testing.T) {
	app := newTestApp()
	router := NewRouter(&mockApplication{}, app)

	handlerRan := false
	router.Use(func(ctx *Context, next func() error) error {
		if err := ctx.AbortWithError(401, fmt.Errorf("no token")); err != nil {
			return err
		}
		return next()
	})
	router.GET("/", func(ctx *Context) error {
		handlerRan = true
		return ctx.String("should not run")
	})

	resp, err := app.Test(httptest.NewRequest("GET", "/", nil), -1)
	require.NoError(t, err)

	assert.Equal(t, 401, resp.StatusCode)
	assert.False(t, handlerRan)
}

// --- File / Download ---

func TestContextFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "notes.txt")
	require.NoError(t, os.WriteFile(path, []byte("the quick brown fox"), 0o600))

	resp, body := runHandler(t, func(ctx *Context) error {
		return ctx.File(path)
	})

	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, "the quick brown fox", body)
	assert.Empty(t, resp.Header.Get("Content-Disposition"))
}

func TestContextFileMissing(t *testing.T) {
	resp, _ := runHandler(t, func(ctx *Context) error {
		return ctx.File(filepath.Join(t.TempDir(), "absent.txt"))
	})

	assert.Equal(t, 404, resp.StatusCode)
}

func TestContextDownload(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "report.csv")
	require.NoError(t, os.WriteFile(path, []byte("id,name\n1,Ada\n"), 0o600))

	resp, body := runHandler(t, func(ctx *Context) error {
		return ctx.Download(path)
	})

	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, "id,name\n1,Ada\n", body)
	assert.Equal(t, `attachment; filename="report.csv"`, resp.Header.Get("Content-Disposition"))
}

func TestContextDownloadWithExplicitName(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tmp-98f2c1.csv")
	require.NoError(t, os.WriteFile(path, []byte("id\n1\n"), 0o600))

	resp, _ := runHandler(t, func(ctx *Context) error {
		return ctx.Download(path, "sales-2024.csv")
	})

	assert.Equal(t, `attachment; filename="sales-2024.csv"`, resp.Header.Get("Content-Disposition"))
}

// --- Resource envelopes ---

// ResourceWith carries metadata beside the data envelope, for a listing
// that has to report more than its rows.
func TestContextResourceWith(t *testing.T) {
	resp, body := runHandler(t, func(ctx *Context) error {
		return ctx.ResourceWith(
			[]map[string]any{{"id": 1}},
			map[string]any{"total": 1, "per_page": 15},
		)
	})

	assert.Equal(t, 200, resp.StatusCode)
	assert.JSONEq(t, `{"data":[{"id":1}],"meta":{"total":1,"per_page":15}}`, body)
}

// Nil metadata still produces the meta key, so a client can read it
// unconditionally.
func TestContextResourceWithNilMeta(t *testing.T) {
	_, body := runHandler(t, func(ctx *Context) error {
		return ctx.ResourceWith(map[string]any{"id": 1}, nil)
	})

	assert.JSONEq(t, `{"data":{"id":1},"meta":null}`, body)
}
