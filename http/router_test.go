package http

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/genesysflow/go-genesys/contracts"
	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockApplication is a minimal mock for contracts.Application
type mockApplication struct{}

func (m *mockApplication) Version() string                                   { return "test" }
func (m *mockApplication) BasePath() string                                  { return "/" }
func (m *mockApplication) SetBasePath(path string) contracts.Application     { return m }
func (m *mockApplication) ConfigPath() string                                { return "/config" }
func (m *mockApplication) StoragePath() string                               { return "/storage" }
func (m *mockApplication) Environment() string                               { return "testing" }
func (m *mockApplication) IsEnvironment(envs ...string) bool                 { return true }
func (m *mockApplication) IsProduction() bool                                { return false }
func (m *mockApplication) IsLocal() bool                                     { return true }
func (m *mockApplication) IsDebug() bool                                     { return true }
func (m *mockApplication) Bind(key string, resolver any) error               { return nil }
func (m *mockApplication) Singleton(key string, resolver any) error          { return nil }
func (m *mockApplication) BindValue(key string, value any) error             { return nil }
func (m *mockApplication) BindType(resolver any) error                       { return nil }
func (m *mockApplication) SingletonType(resolver any) error                  { return nil }
func (m *mockApplication) Instance(key string, instance any) error           { return nil }
func (m *mockApplication) InstanceType(instance any) error                   { return nil }
func (m *mockApplication) Make(key string) (any, error)                      { return nil, nil }
func (m *mockApplication) MustMake(key string) any                           { return nil }
func (m *mockApplication) Has(key string) bool                               { return false }
func (m *mockApplication) Shutdown() error                                   { return nil }
func (m *mockApplication) ShutdownWithContext(ctx context.Context) error     { return nil }
func (m *mockApplication) Register(provider contracts.ServiceProvider) error { return nil }
func (m *mockApplication) Boot() error                                       { return nil }
func (m *mockApplication) IsBooted() bool                                    { return true }
func (m *mockApplication) Booting(fn func(contracts.Application))            {}
func (m *mockApplication) Booted(fn func(contracts.Application))             {}
func (m *mockApplication) Terminating(fn func(contracts.Application))        {}
func (m *mockApplication) Terminate() error                                  { return nil }
func (m *mockApplication) TerminateWithContext(ctx context.Context) error    { return nil }
func (m *mockApplication) GetConfig() contracts.Config                       { return nil }
func (m *mockApplication) GetLogger() contracts.Logger                       { return nil }

func newTestApp() *fiber.App {
	return fiber.New(fiber.Config{
		DisableStartupMessage: true,
	})
}

func TestNewRouter(t *testing.T) {
	app := newTestApp()
	mockApp := &mockApplication{}

	router := NewRouter(mockApp, app)
	assert.NotNil(t, router)
	assert.Equal(t, mockApp, router.App())
}

func TestRouterGET(t *testing.T) {
	app := newTestApp()
	router := NewRouter(&mockApplication{}, app)

	router.GET("/test", func(ctx *Context) error {
		return ctx.String("GET response")
	})

	req := httptest.NewRequest("GET", "/test", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	assert.Equal(t, "GET response", string(body))
}

func TestRouterPOST(t *testing.T) {
	app := newTestApp()
	router := NewRouter(&mockApplication{}, app)

	router.POST("/test", func(ctx *Context) error {
		return ctx.String("POST response")
	})

	req := httptest.NewRequest("POST", "/test", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestRouterPUT(t *testing.T) {
	app := newTestApp()
	router := NewRouter(&mockApplication{}, app)

	router.PUT("/test", func(ctx *Context) error {
		return ctx.String("PUT response")
	})

	req := httptest.NewRequest("PUT", "/test", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestRouterPATCH(t *testing.T) {
	app := newTestApp()
	router := NewRouter(&mockApplication{}, app)

	router.PATCH("/test", func(ctx *Context) error {
		return ctx.String("PATCH response")
	})

	req := httptest.NewRequest("PATCH", "/test", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestRouterDELETE(t *testing.T) {
	app := newTestApp()
	router := NewRouter(&mockApplication{}, app)

	router.DELETE("/test", func(ctx *Context) error {
		return ctx.String("DELETE response")
	})

	req := httptest.NewRequest("DELETE", "/test", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestRouterOPTIONS(t *testing.T) {
	app := newTestApp()
	router := NewRouter(&mockApplication{}, app)

	router.OPTIONS("/test", func(ctx *Context) error {
		return ctx.String("OPTIONS response")
	})

	req := httptest.NewRequest("OPTIONS", "/test", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestRouterHEAD(t *testing.T) {
	app := newTestApp()
	router := NewRouter(&mockApplication{}, app)

	router.HEAD("/test", func(ctx *Context) error {
		ctx.Header("X-Custom", "value")
		return nil
	})

	req := httptest.NewRequest("HEAD", "/test", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestRouterAny(t *testing.T) {
	app := newTestApp()
	router := NewRouter(&mockApplication{}, app)

	router.Any("/any", func(ctx *Context) error {
		return ctx.String(ctx.Request().Method())
	})

	methods := []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS", "HEAD"}
	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/any", nil)
			resp, err := app.Test(req)
			require.NoError(t, err)
			assert.Equal(t, 200, resp.StatusCode)
		})
	}
}

func TestRouterMatch(t *testing.T) {
	app := newTestApp()
	router := NewRouter(&mockApplication{}, app)

	router.Match([]string{"GET", "POST"}, "/match", func(ctx *Context) error {
		return ctx.String("matched")
	})

	// Should work for GET and POST
	req := httptest.NewRequest("GET", "/match", nil)
	resp, _ := app.Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	req = httptest.NewRequest("POST", "/match", nil)
	resp, _ = app.Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	// Should 405 Method Not Allowed for other methods (since route exists but for different method)
	req = httptest.NewRequest("PUT", "/match", nil)
	resp, _ = app.Test(req)
	assert.Equal(t, 405, resp.StatusCode)
}

func TestRouterGroup(t *testing.T) {
	app := newTestApp()
	router := NewRouter(&mockApplication{}, app)

	router.Group("/api", func(r *Router) {
		r.GET("/users", func(ctx *Context) error {
			return ctx.String("users")
		})
		r.GET("/posts", func(ctx *Context) error {
			return ctx.String("posts")
		})
	})

	req := httptest.NewRequest("GET", "/api/users", nil)
	resp, _ := app.Test(req)
	assert.Equal(t, 200, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	assert.Equal(t, "users", string(body))

	req = httptest.NewRequest("GET", "/api/posts", nil)
	resp, _ = app.Test(req)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestRouterNestedGroups(t *testing.T) {
	app := newTestApp()
	router := NewRouter(&mockApplication{}, app)

	router.Group("/api", func(r *Router) {
		r.Group("/v1", func(r2 *Router) {
			r2.GET("/users", func(ctx *Context) error {
				return ctx.String("v1 users")
			})
		})
	})

	req := httptest.NewRequest("GET", "/api/v1/users", nil)
	resp, _ := app.Test(req)
	assert.Equal(t, 200, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	assert.Equal(t, "v1 users", string(body))
}

func TestRouterMiddleware(t *testing.T) {
	app := newTestApp()
	router := NewRouter(&mockApplication{}, app)

	var middlewareExecuted bool
	middleware := func(ctx *Context, next func() error) error {
		middlewareExecuted = true
		return next()
	}

	router.GET("/test", func(ctx *Context) error {
		return ctx.String("response")
	}, middleware)

	req := httptest.NewRequest("GET", "/test", nil)
	_, err := app.Test(req)
	require.NoError(t, err)
	assert.True(t, middlewareExecuted)
}

func TestRouterGroupMiddleware(t *testing.T) {
	app := newTestApp()
	router := NewRouter(&mockApplication{}, app)

	var callOrder []string
	groupMiddleware := func(ctx *Context, next func() error) error {
		callOrder = append(callOrder, "group")
		return next()
	}
	routeMiddleware := func(ctx *Context, next func() error) error {
		callOrder = append(callOrder, "route")
		return next()
	}

	router.Group("/api", func(r *Router) {
		r.GET("/test", func(ctx *Context) error {
			callOrder = append(callOrder, "handler")
			return ctx.String("response")
		}, routeMiddleware)
	}, groupMiddleware)

	req := httptest.NewRequest("GET", "/api/test", nil)
	_, _ = app.Test(req)

	assert.Equal(t, []string{"group", "route", "handler"}, callOrder)
}

func TestRouterUse(t *testing.T) {
	app := newTestApp()
	router := NewRouter(&mockApplication{}, app)

	var middlewareExecuted bool
	router.Use(func(ctx *Context, next func() error) error {
		middlewareExecuted = true
		return next()
	})

	router.GET("/test", func(ctx *Context) error {
		return ctx.String("response")
	})

	req := httptest.NewRequest("GET", "/test", nil)
	_, _ = app.Test(req)
	assert.True(t, middlewareExecuted)
}

func TestRouteParams(t *testing.T) {
	app := newTestApp()
	router := NewRouter(&mockApplication{}, app)

	router.GET("/users/:id", func(ctx *Context) error {
		id := ctx.Param("id")
		return ctx.String("user: " + id)
	})

	req := httptest.NewRequest("GET", "/users/123", nil)
	resp, _ := app.Test(req)
	body, _ := io.ReadAll(resp.Body)
	assert.Equal(t, "user: 123", string(body))
}

func TestRouteParamsInt(t *testing.T) {
	app := newTestApp()
	router := NewRouter(&mockApplication{}, app)

	router.GET("/users/:id", func(ctx *Context) error {
		id := ctx.ParamInt("id")
		return ctx.JSONResponse(map[string]int{"id": id})
	})

	req := httptest.NewRequest("GET", "/users/456", nil)
	resp, _ := app.Test(req)
	body, _ := io.ReadAll(resp.Body)
	assert.Contains(t, string(body), "456")
}

func TestQueryParams(t *testing.T) {
	app := newTestApp()
	router := NewRouter(&mockApplication{}, app)

	router.GET("/search", func(ctx *Context) error {
		q := ctx.Query("q")
		return ctx.String("query: " + q)
	})

	req := httptest.NewRequest("GET", "/search?q=hello", nil)
	resp, _ := app.Test(req)
	body, _ := io.ReadAll(resp.Body)
	assert.Equal(t, "query: hello", string(body))
}

func TestQueryParamsDefault(t *testing.T) {
	app := newTestApp()
	router := NewRouter(&mockApplication{}, app)

	router.GET("/search", func(ctx *Context) error {
		q := ctx.Query("q", "default")
		return ctx.String("query: " + q)
	})

	req := httptest.NewRequest("GET", "/search", nil)
	resp, _ := app.Test(req)
	body, _ := io.ReadAll(resp.Body)
	assert.Equal(t, "query: default", string(body))
}

func TestRouteName(t *testing.T) {
	app := newTestApp()
	router := NewRouter(&mockApplication{}, app)

	route := router.GET("/users", func(ctx *Context) error {
		return ctx.String("users")
	}).Name("users.index")

	assert.Equal(t, "users.index", route.GetName())
	assert.Equal(t, "/users", route.GetPath())
	assert.Equal(t, "GET", route.GetMethod())

	// Should be retrievable by name
	namedRoute := router.NamedRoute("users.index")
	assert.NotNil(t, namedRoute)
	assert.Equal(t, route, namedRoute)
}

func TestRoutes(t *testing.T) {
	app := newTestApp()
	router := NewRouter(&mockApplication{}, app)

	router.GET("/one", func(ctx *Context) error { return nil })
	router.POST("/two", func(ctx *Context) error { return nil })
	router.PUT("/three", func(ctx *Context) error { return nil })

	routes := router.Routes()
	assert.Len(t, routes, 3)
}

func Test404NotFound(t *testing.T) {
	app := newTestApp()
	router := NewRouter(&mockApplication{}, app)

	router.GET("/exists", func(ctx *Context) error {
		return ctx.String("exists")
	})

	req := httptest.NewRequest("GET", "/not-exists", nil)
	resp, _ := app.Test(req)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestStatic(t *testing.T) {
	app := newTestApp()
	router := NewRouter(&mockApplication{}, app)

	// Static routes don't error if path doesn't exist during setup
	router.Static("/static", "./testdata")

	// Request would return 404 if testdata doesn't exist, but route is registered
	req := httptest.NewRequest("GET", "/static/nonexistent.txt", nil)
	resp, _ := app.Test(req)
	// Will be 404 since testdata doesn't exist, but that's expected
	assert.NotNil(t, resp)
}
