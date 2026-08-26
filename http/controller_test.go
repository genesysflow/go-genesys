package http

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// serveContext drives httpReq through a handler that receives the
// framework's Context, and returns the response with its body.
func serveContext(t *testing.T, method, route string, httpReq *http.Request, handler HandlerFunc) (*http.Response, string) {
	t.Helper()

	app := newTestApp()
	router := NewRouter(&mockApplication{}, app)
	router.addRoute(method, route, handler)

	resp, err := app.Test(httpReq, -1)
	require.NoError(t, err)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())

	return resp, string(body)
}

// runHandler drives a plain GET / through the handler and returns the
// response with its raw body.
func runHandler(t *testing.T, handler HandlerFunc) (*http.Response, string) {
	t.Helper()
	return serveContext(t, "GET", "/", httptest.NewRequest("GET", "/", nil), handler)
}

// serveGetJSON is runHandler for a handler that answers with JSON.
func serveGetJSON(t *testing.T, handler HandlerFunc) (*http.Response, map[string]any) {
	t.Helper()

	resp, body := serveContext(t, "GET", "/", httptest.NewRequest("GET", "/", nil), handler)

	var decoded map[string]any
	if body != "" {
		require.NoError(t, json.Unmarshal([]byte(body), &decoded), "body was %q", body)
	}
	return resp, decoded
}

// --- construction ---

func TestNewController(t *testing.T) {
	app := &mockApplication{}
	controller := NewController(app)

	require.NotNil(t, controller)
	assert.Same(t, app, controller.App())
}

func TestControllerSetApp(t *testing.T) {
	controller := NewController(nil)
	assert.Nil(t, controller.App())

	replacement := &mockApplication{}
	controller.SetApp(replacement)
	assert.Same(t, replacement, controller.App())
}

// A zero Controller embedded in a user's own type is usable before an
// app is set, which is what makes `type PostController struct { http.Controller }`
// work.
func TestControllerZeroValue(t *testing.T) {
	var controller Controller
	assert.Nil(t, controller.App())
}

// --- response helpers ---

func TestControllerJSON(t *testing.T) {
	controller := NewController(&mockApplication{})

	resp, decoded := serveGetJSON(t, func(ctx *Context) error {
		return controller.JSON(ctx, map[string]any{"name": "Ada"})
	})

	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, map[string]any{"name": "Ada"}, decoded)
}

func TestControllerSuccess(t *testing.T) {
	controller := NewController(&mockApplication{})

	resp, decoded := serveGetJSON(t, func(ctx *Context) error {
		return controller.Success(ctx, []string{"a", "b"})
	})

	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, true, decoded["success"])
	assert.Equal(t, []any{"a", "b"}, decoded["data"])
	assert.NotContains(t, decoded, "message", "no message unless one is given")
}

func TestControllerSuccessWithMessage(t *testing.T) {
	controller := NewController(&mockApplication{})

	_, decoded := serveGetJSON(t, func(ctx *Context) error {
		return controller.Success(ctx, nil, "Saved.", "ignored second message")
	})

	assert.Equal(t, "Saved.", decoded["message"], "only the first message is used")
	assert.Nil(t, decoded["data"])
}

func TestControllerError(t *testing.T) {
	controller := NewController(&mockApplication{})

	resp, decoded := serveGetJSON(t, func(ctx *Context) error {
		return controller.Error(ctx, 418, "I am a teapot")
	})

	assert.Equal(t, 418, resp.StatusCode)
	assert.Equal(t, false, decoded["success"])
	assert.Equal(t, "I am a teapot", decoded["error"])
	assert.NotContains(t, decoded, "errors")
}

func TestControllerErrorWithDetails(t *testing.T) {
	controller := NewController(&mockApplication{})

	_, decoded := serveGetJSON(t, func(ctx *Context) error {
		return controller.Error(ctx, 400, "Bad input", map[string]any{"name": "required"})
	})

	assert.Equal(t, map[string]any{"name": "required"}, decoded["errors"])
}

func TestControllerCreated(t *testing.T) {
	controller := NewController(&mockApplication{})

	resp, decoded := serveGetJSON(t, func(ctx *Context) error {
		return controller.Created(ctx, map[string]any{"id": float64(7)}, "Created.")
	})

	assert.Equal(t, 201, resp.StatusCode)
	assert.Equal(t, true, decoded["success"])
	assert.Equal(t, map[string]any{"id": float64(7)}, decoded["data"])
	assert.Equal(t, "Created.", decoded["message"])
}

func TestControllerCreatedWithoutMessage(t *testing.T) {
	controller := NewController(&mockApplication{})

	resp, decoded := serveGetJSON(t, func(ctx *Context) error {
		return controller.Created(ctx, nil)
	})

	assert.Equal(t, 201, resp.StatusCode)
	assert.NotContains(t, decoded, "message")
}

func TestControllerNoContent(t *testing.T) {
	controller := NewController(&mockApplication{})

	resp, body := serveContext(t, "GET", "/", httptest.NewRequest("GET", "/", nil), func(ctx *Context) error {
		return controller.NoContent(ctx)
	})

	assert.Equal(t, 204, resp.StatusCode)
	assert.Empty(t, body)
}

// The status-shaped helpers all funnel through Error, and each has a
// default message the caller can replace.
func TestControllerStatusHelpers(t *testing.T) {
	controller := NewController(&mockApplication{})

	cases := []struct {
		name        string
		call        func(*Context) error
		wantStatus  int
		wantMessage string
	}{
		{
			name:        "NotFound default",
			call:        func(ctx *Context) error { return controller.NotFound(ctx) },
			wantStatus:  404,
			wantMessage: "Resource not found",
		},
		{
			name:        "NotFound custom",
			call:        func(ctx *Context) error { return controller.NotFound(ctx, "No such post") },
			wantStatus:  404,
			wantMessage: "No such post",
		},
		{
			name:        "Unauthorized default",
			call:        func(ctx *Context) error { return controller.Unauthorized(ctx) },
			wantStatus:  401,
			wantMessage: "Unauthorized",
		},
		{
			name:        "Unauthorized custom",
			call:        func(ctx *Context) error { return controller.Unauthorized(ctx, "Token expired") },
			wantStatus:  401,
			wantMessage: "Token expired",
		},
		{
			name:        "Forbidden default",
			call:        func(ctx *Context) error { return controller.Forbidden(ctx) },
			wantStatus:  403,
			wantMessage: "Forbidden",
		},
		{
			name:        "Forbidden custom",
			call:        func(ctx *Context) error { return controller.Forbidden(ctx, "Not your post") },
			wantStatus:  403,
			wantMessage: "Not your post",
		},
		{
			name:        "BadRequest",
			call:        func(ctx *Context) error { return controller.BadRequest(ctx, "Malformed cursor") },
			wantStatus:  400,
			wantMessage: "Malformed cursor",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp, decoded := serveGetJSON(t, tc.call)

			assert.Equal(t, tc.wantStatus, resp.StatusCode)
			assert.Equal(t, false, decoded["success"])
			assert.Equal(t, tc.wantMessage, decoded["error"])
		})
	}
}

func TestControllerBadRequestWithDetails(t *testing.T) {
	controller := NewController(&mockApplication{})

	resp, decoded := serveGetJSON(t, func(ctx *Context) error {
		return controller.BadRequest(ctx, "Malformed cursor", []string{"cursor"})
	})

	assert.Equal(t, 400, resp.StatusCode)
	assert.Equal(t, []any{"cursor"}, decoded["errors"])
}

func TestControllerValidationError(t *testing.T) {
	controller := NewController(&mockApplication{})

	resp, decoded := serveGetJSON(t, func(ctx *Context) error {
		return controller.ValidationError(ctx, map[string][]string{"email": {"is required"}})
	})

	assert.Equal(t, 422, resp.StatusCode)
	assert.Equal(t, false, decoded["success"])
	assert.Equal(t, "Validation failed", decoded["error"])
	assert.Equal(t, map[string]any{"email": []any{"is required"}}, decoded["errors"])
}

// --- pagination meta ---

func TestControllerPaginate(t *testing.T) {
	controller := NewController(&mockApplication{})

	cases := []struct {
		name                        string
		page, perPage, total        int
		wantTotalPages, wantHasMore any
	}{
		{"middle page", 2, 10, 95, float64(10), true},
		{"last page", 10, 10, 95, float64(10), false},
		{"exact multiple", 1, 10, 20, float64(2), true},
		{"single page", 1, 10, 3, float64(1), false},
		{"empty result set", 1, 10, 0, float64(0), false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp, decoded := serveGetJSON(t, func(ctx *Context) error {
				return controller.Paginate(ctx, []string{"a"}, tc.page, tc.perPage, tc.total)
			})

			assert.Equal(t, 200, resp.StatusCode)
			assert.Equal(t, true, decoded["success"])
			assert.Equal(t, []any{"a"}, decoded["data"])

			meta, ok := decoded["meta"].(map[string]any)
			require.True(t, ok, "meta should be an object, got %#v", decoded["meta"])
			assert.Equal(t, float64(tc.page), meta["page"])
			assert.Equal(t, float64(tc.perPage), meta["per_page"])
			assert.Equal(t, float64(tc.total), meta["total"])
			assert.Equal(t, tc.wantTotalPages, meta["total_pages"])
			assert.Equal(t, tc.wantHasMore, meta["has_more"])
		})
	}
}
