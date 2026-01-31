package http

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
)

func TestNewRequest(t *testing.T) {
	app := fiber.New()
	app.Get("/test", func(c *fiber.Ctx) error {
		req := NewRequest(c)
		assert.NotNil(t, req)
		assert.Equal(t, c, req.FiberCtx())
		return nil
	})

	httpReq := httptest.NewRequest("GET", "/test", nil)
	_, _ = app.Test(httpReq)
}

func TestRequestMethod(t *testing.T) {
	methods := []string{"GET", "POST", "PUT", "PATCH", "DELETE"}

	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			app := fiber.New()
			app.Add(method, "/test", func(c *fiber.Ctx) error {
				req := NewRequest(c)
				assert.Equal(t, method, req.Method())
				return nil
			})

			httpReq := httptest.NewRequest(method, "/test", nil)
			_, _ = app.Test(httpReq)
		})
	}
}

func TestRequestPath(t *testing.T) {
	app := fiber.New()
	app.Get("/api/v1/users", func(c *fiber.Ctx) error {
		req := NewRequest(c)
		assert.Equal(t, "/api/v1/users", req.Path())
		return nil
	})

	httpReq := httptest.NewRequest("GET", "/api/v1/users", nil)
	_, _ = app.Test(httpReq)
}

func TestRequestURI(t *testing.T) {
	app := fiber.New()
	app.Get("/search", func(c *fiber.Ctx) error {
		req := NewRequest(c)
		assert.Equal(t, "/search?q=test&page=1", req.URI())
		return nil
	})

	httpReq := httptest.NewRequest("GET", "/search?q=test&page=1", nil)
	_, _ = app.Test(httpReq)
}

func TestRequestHost(t *testing.T) {
	app := fiber.New()
	app.Get("/test", func(c *fiber.Ctx) error {
		req := NewRequest(c)
		assert.Equal(t, "example.com", req.Host())
		return nil
	})

	httpReq := httptest.NewRequest("GET", "/test", nil)
	httpReq.Host = "example.com"
	_, _ = app.Test(httpReq)
}

func TestRequestIP(t *testing.T) {
	app := fiber.New()
	app.Get("/test", func(c *fiber.Ctx) error {
		req := NewRequest(c)
		// In test, IP is typically 0.0.0.0
		ip := req.IP()
		assert.NotEmpty(t, ip)
		return nil
	})

	httpReq := httptest.NewRequest("GET", "/test", nil)
	_, _ = app.Test(httpReq)
}

func TestRequestHeader(t *testing.T) {
	app := fiber.New()
	app.Get("/test", func(c *fiber.Ctx) error {
		req := NewRequest(c)
		assert.Equal(t, "application/json", req.Header("Accept"))
		assert.Equal(t, "Bearer token123", req.Header("Authorization"))
		assert.Equal(t, "", req.Header("X-Nonexistent"))
		return nil
	})

	httpReq := httptest.NewRequest("GET", "/test", nil)
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("Authorization", "Bearer token123")
	_, _ = app.Test(httpReq)
}

func TestRequestHeaders(t *testing.T) {
	app := fiber.New()
	app.Get("/test", func(c *fiber.Ctx) error {
		req := NewRequest(c)
		headers := req.Headers()
		assert.NotEmpty(t, headers)
		assert.Equal(t, "custom-value", headers["X-Custom-Header"])
		return nil
	})

	httpReq := httptest.NewRequest("GET", "/test", nil)
	httpReq.Header.Set("X-Custom-Header", "custom-value")
	_, _ = app.Test(httpReq)
}

func TestRequestQuery(t *testing.T) {
	app := fiber.New()
	app.Get("/search", func(c *fiber.Ctx) error {
		req := NewRequest(c)
		assert.Equal(t, "hello", req.Query("q"))
		assert.Equal(t, "1", req.Query("page"))
		assert.Equal(t, "", req.Query("missing"))
		assert.Equal(t, "default", req.Query("missing", "default"))
		return nil
	})

	httpReq := httptest.NewRequest("GET", "/search?q=hello&page=1", nil)
	_, _ = app.Test(httpReq)
}

func TestRequestQueryInt(t *testing.T) {
	app := fiber.New()
	app.Get("/page", func(c *fiber.Ctx) error {
		req := NewRequest(c)
		assert.Equal(t, 5, req.QueryInt("page"))
		assert.Equal(t, 100, req.QueryInt("limit"))
		assert.Equal(t, 0, req.QueryInt("missing"))
		assert.Equal(t, 10, req.QueryInt("missing", 10))
		assert.Equal(t, 0, req.QueryInt("invalid")) // "abc" -> 0
		return nil
	})

	httpReq := httptest.NewRequest("GET", "/page?page=5&limit=100&invalid=abc", nil)
	_, _ = app.Test(httpReq)
}

func TestRequestParam(t *testing.T) {
	app := fiber.New()
	app.Get("/users/:id/posts/:postId", func(c *fiber.Ctx) error {
		req := NewRequest(c)
		assert.Equal(t, "123", req.Param("id"))
		assert.Equal(t, "456", req.Param("postId"))
		assert.Equal(t, "", req.Param("missing"))
		assert.Equal(t, "default", req.Param("missing", "default"))
		return nil
	})

	httpReq := httptest.NewRequest("GET", "/users/123/posts/456", nil)
	_, _ = app.Test(httpReq)
}

func TestRequestParamInt(t *testing.T) {
	app := fiber.New()
	app.Get("/users/:id", func(c *fiber.Ctx) error {
		req := NewRequest(c)
		assert.Equal(t, 789, req.ParamInt("id"))
		assert.Equal(t, 0, req.ParamInt("missing"))
		assert.Equal(t, 99, req.ParamInt("missing", 99))
		return nil
	})

	httpReq := httptest.NewRequest("GET", "/users/789", nil)
	_, _ = app.Test(httpReq)
}

func TestRequestInput(t *testing.T) {
	app := fiber.New()
	app.Get("/test/:id", func(c *fiber.Ctx) error {
		req := NewRequest(c)
		// Input checks params first, then query, then form
		assert.Equal(t, "param-value", req.Input("id"))     // from params
		assert.Equal(t, "query-value", req.Input("search")) // from query
		assert.Equal(t, "", req.Input("missing"))
		assert.Equal(t, "default", req.Input("missing", "default"))
		return nil
	})

	httpReq := httptest.NewRequest("GET", "/test/param-value?search=query-value", nil)
	_, _ = app.Test(httpReq)
}

func TestRequestAll(t *testing.T) {
	app := fiber.New()
	app.Get("/test", func(c *fiber.Ctx) error {
		req := NewRequest(c)
		all := req.All()
		assert.Equal(t, "value1", all["key1"])
		assert.Equal(t, "value2", all["key2"])
		return nil
	})

	httpReq := httptest.NewRequest("GET", "/test?key1=value1&key2=value2", nil)
	_, _ = app.Test(httpReq)
}

func TestRequestJSON(t *testing.T) {
	app := fiber.New()
	app.Post("/json", func(c *fiber.Ctx) error {
		req := NewRequest(c)
		var data map[string]string
		err := req.JSON(&data)
		assert.NoError(t, err)
		assert.Equal(t, "John", data["name"])
		return nil
	})

	body := `{"name":"John"}`
	httpReq := httptest.NewRequest("POST", "/json", strings.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	_, _ = app.Test(httpReq)
}

func TestRequestContext(t *testing.T) {
	app := fiber.New()
	app.Get("/test", func(c *fiber.Ctx) error {
		req := NewRequest(c)
		ctx := req.Context()
		assert.NotNil(t, ctx)
		return nil
	})

	httpReq := httptest.NewRequest("GET", "/test", nil)
	_, _ = app.Test(httpReq)
}

func TestRequestWithContext(t *testing.T) {
	app := fiber.New()
	app.Get("/test", func(c *fiber.Ctx) error {
		req := NewRequest(c)
		ctx := context.WithValue(context.Background(), "key", "value")
		newReq := req.WithContext(ctx)
		assert.NotNil(t, newReq)
		return nil
	})

	httpReq := httptest.NewRequest("GET", "/test", nil)
	_, _ = app.Test(httpReq)
}

func TestRequestScheme(t *testing.T) {
	app := fiber.New()
	app.Get("/test", func(c *fiber.Ctx) error {
		req := NewRequest(c)
		scheme := req.Scheme()
		assert.Equal(t, "http", scheme)
		return nil
	})

	httpReq := httptest.NewRequest("GET", "/test", nil)
	_, _ = app.Test(httpReq)
}

func TestRequestIsSecure(t *testing.T) {
	app := fiber.New()
	app.Get("/test", func(c *fiber.Ctx) error {
		req := NewRequest(c)
		// In tests, requests are typically not secure
		assert.False(t, req.IsSecure())
		return nil
	})

	httpReq := httptest.NewRequest("GET", "/test", nil)
	_, _ = app.Test(httpReq)
}
