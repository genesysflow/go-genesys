package http

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recordingController answers every action with the action's own name,
// so a request can be traced back to the method that served it.
type recordingController struct{ called []string }

func (c *recordingController) answer(action string) HandlerFunc {
	return func(ctx *Context) error {
		c.called = append(c.called, action)
		return ctx.String(action)
	}
}

func (c *recordingController) Index(ctx *Context) error  { return c.answer("index")(ctx) }
func (c *recordingController) Create(ctx *Context) error { return c.answer("create")(ctx) }
func (c *recordingController) Store(ctx *Context) error  { return c.answer("store")(ctx) }
func (c *recordingController) Show(ctx *Context) error {
	return c.answer("show:" + ctx.Param("id"))(ctx)
}
func (c *recordingController) Edit(ctx *Context) error {
	return c.answer("edit:" + ctx.Param("id"))(ctx)
}
func (c *recordingController) Update(ctx *Context) error {
	return c.answer("update:" + ctx.Param("id"))(ctx)
}
func (c *recordingController) Destroy(ctx *Context) error {
	return c.answer("destroy:" + ctx.Param("id"))(ctx)
}

// call drives one request through the app and returns status and body.
func call(t *testing.T, app *fiber.App, method, target string) (int, string) {
	t.Helper()

	resp, err := app.Test(httptest.NewRequest(method, target, nil), -1)
	require.NoError(t, err)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())

	return resp.StatusCode, string(body)
}

// --- Route.GetHandler ---

// GetHandler returns the handler the route was registered with, which is
// what lets a route table be introspected.
func TestRouteGetHandler(t *testing.T) {
	app := newTestApp()
	router := NewRouter(&mockApplication{}, app)

	handler := func(ctx *Context) error { return ctx.String("handled") }
	route := router.GET("/things", handler)

	require.NotNil(t, route.GetHandler())

	// A func value cannot be compared, so call it and check what it does.
	_, body := serveContext(t, "GET", "/probe", httptest.NewRequest("GET", "/probe", nil), route.GetHandler())
	assert.Equal(t, "handled", body)
}

// --- Router.Resource ---

// Resource lays down the seven RESTful routes a browser-facing resource
// needs, each under Laravel's naming convention.
func TestRouterResourceRegistersEveryAction(t *testing.T) {
	app := newTestApp()
	router := NewRouter(&mockApplication{}, app)
	controller := &recordingController{}

	router.Resource("posts", controller)

	cases := []struct {
		name   string
		method string
		path   string
		want   string
	}{
		{"posts.index", "GET", "/posts", "index"},
		{"posts.create", "GET", "/posts/create", "create"},
		{"posts.store", "POST", "/posts", "store"},
		{"posts.show", "GET", "/posts/7", "show:7"},
		{"posts.edit", "GET", "/posts/7/edit", "edit:7"},
		{"posts.update", "PUT", "/posts/7", "update:7"},
		{"posts.update.patch", "PATCH", "/posts/7", "update:7"},
		{"posts.destroy", "DELETE", "/posts/7", "destroy:7"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			route := router.NamedRoute(tc.name)
			require.NotNil(t, route, "route %s should be named", tc.name)
			assert.Equal(t, tc.method, route.GetMethod())

			status, body := call(t, app, tc.method, tc.path)
			assert.Equal(t, 200, status)
			assert.Equal(t, tc.want, body)
		})
	}

	assert.Equal(t, []string{
		"index", "create", "store", "show:7", "edit:7", "update:7", "update:7", "destroy:7",
	}, controller.called)
}

// The named routes resolve to URLs, which is what a template's route
// helper needs.
func TestRouterResourceNamesResolveToURLs(t *testing.T) {
	app := newTestApp()
	router := NewRouter(&mockApplication{}, app)
	router.Resource("posts", &recordingController{})

	assert.Equal(t, "/posts", router.URL("posts.index"))
	assert.Equal(t, "/posts/create", router.URL("posts.create"))
	assert.Equal(t, "/posts/7", router.URL("posts.show", map[string]any{"id": 7}))
	assert.Equal(t, "/posts/7/edit", router.URL("posts.edit", map[string]any{"id": 7}))
}

// A resource registered inside a group inherits the prefix.
func TestRouterResourceInsideGroup(t *testing.T) {
	app := newTestApp()
	router := NewRouter(&mockApplication{}, app)

	router.Group("/admin", func(admin *Router) {
		admin.Resource("posts", &recordingController{})
	})

	status, body := call(t, app, "GET", "/admin/posts")
	assert.Equal(t, 200, status)
	assert.Equal(t, "index", body)
}

// --- Router.APIResource ---

// APIResource omits create and edit: an API client has no forms to fill.
func TestRouterAPIResourceRegistersEveryAction(t *testing.T) {
	app := newTestApp()
	router := NewRouter(&mockApplication{}, app)
	controller := &recordingController{}

	router.APIResource("posts", controller)

	cases := []struct {
		name   string
		method string
		path   string
		want   string
	}{
		{"posts.index", "GET", "/posts", "index"},
		{"posts.store", "POST", "/posts", "store"},
		{"posts.show", "GET", "/posts/7", "show:7"},
		{"posts.update", "PUT", "/posts/7", "update:7"},
		{"posts.update.patch", "PATCH", "/posts/7", "update:7"},
		{"posts.destroy", "DELETE", "/posts/7", "destroy:7"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.NotNil(t, router.NamedRoute(tc.name))

			status, body := call(t, app, tc.method, tc.path)
			assert.Equal(t, 200, status)
			assert.Equal(t, tc.want, body)
		})
	}
}

// The form-only routes are absent, so a stray GET /posts/create is a 404
// rather than a silently working page.
func TestRouterAPIResourceOmitsFormRoutes(t *testing.T) {
	app := newTestApp()
	router := NewRouter(&mockApplication{}, app)
	router.APIResource("posts", &recordingController{})

	assert.Nil(t, router.NamedRoute("posts.create"))
	assert.Nil(t, router.NamedRoute("posts.edit"))

	status, _ := call(t, app, "GET", "/posts/7/edit")
	assert.Equal(t, http.StatusNotFound, status)
}
