package http

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewContext(t *testing.T) {
	app := fiber.New()
	app.Get("/test", func(c *fiber.Ctx) error {
		ctx := NewContext(c, &mockApplication{})
		assert.NotNil(t, ctx)
		assert.NotNil(t, ctx.Request())
		assert.NotNil(t, ctx.Response())
		assert.NotNil(t, ctx.App())
		assert.NotNil(t, ctx.FiberCtx())
		return nil
	})

	req := httptest.NewRequest("GET", "/test", nil)
	_, _ = app.Test(req)
}

func TestContextParam(t *testing.T) {
	app := fiber.New()
	app.Get("/users/:id", func(c *fiber.Ctx) error {
		ctx := NewContext(c, &mockApplication{})
		assert.Equal(t, "123", ctx.Param("id"))
		assert.Equal(t, "default", ctx.Param("nonexistent", "default"))
		return nil
	})

	req := httptest.NewRequest("GET", "/users/123", nil)
	_, _ = app.Test(req)
}

func TestContextParamInt(t *testing.T) {
	app := fiber.New()
	app.Get("/users/:id", func(c *fiber.Ctx) error {
		ctx := NewContext(c, &mockApplication{})
		assert.Equal(t, 456, ctx.ParamInt("id"))
		assert.Equal(t, 0, ctx.ParamInt("nonexistent"))
		assert.Equal(t, 99, ctx.ParamInt("nonexistent", 99))
		return nil
	})

	req := httptest.NewRequest("GET", "/users/456", nil)
	_, _ = app.Test(req)
}

func TestContextQuery(t *testing.T) {
	app := fiber.New()
	app.Get("/search", func(c *fiber.Ctx) error {
		ctx := NewContext(c, &mockApplication{})
		assert.Equal(t, "hello", ctx.Query("q"))
		assert.Equal(t, "", ctx.Query("missing"))
		assert.Equal(t, "default", ctx.Query("missing", "default"))
		return nil
	})

	req := httptest.NewRequest("GET", "/search?q=hello", nil)
	_, _ = app.Test(req)
}

func TestContextQueryInt(t *testing.T) {
	app := fiber.New()
	app.Get("/page", func(c *fiber.Ctx) error {
		ctx := NewContext(c, &mockApplication{})
		assert.Equal(t, 5, ctx.QueryInt("page"))
		assert.Equal(t, 0, ctx.QueryInt("missing"))
		assert.Equal(t, 1, ctx.QueryInt("missing", 1))
		return nil
	})

	req := httptest.NewRequest("GET", "/page?page=5", nil)
	_, _ = app.Test(req)
}

func TestContextInput(t *testing.T) {
	app := fiber.New()
	app.Get("/test/:id", func(c *fiber.Ctx) error {
		ctx := NewContext(c, &mockApplication{})
		// Input checks params, query, then form
		assert.Equal(t, "123", ctx.Input("id"))           // from params
		assert.Equal(t, "queryval", ctx.Input("q"))       // from query
		assert.Equal(t, "default", ctx.Input("missing", "default"))
		return nil
	})

	req := httptest.NewRequest("GET", "/test/123?q=queryval", nil)
	_, _ = app.Test(req)
}

func TestContextAll(t *testing.T) {
	app := fiber.New()
	app.Get("/test", func(c *fiber.Ctx) error {
		ctx := NewContext(c, &mockApplication{})
		all := ctx.All()
		assert.NotNil(t, all)
		assert.Equal(t, "value1", all["key1"])
		assert.Equal(t, "value2", all["key2"])
		return nil
	})

	req := httptest.NewRequest("GET", "/test?key1=value1&key2=value2", nil)
	_, _ = app.Test(req)
}

func TestContextJSON(t *testing.T) {
	app := fiber.New()
	app.Post("/json", func(c *fiber.Ctx) error {
		ctx := NewContext(c, &mockApplication{})
		var data struct {
			Name  string `json:"name"`
			Email string `json:"email"`
		}
		err := ctx.JSON(&data)
		require.NoError(t, err)
		assert.Equal(t, "John", data.Name)
		assert.Equal(t, "john@example.com", data.Email)
		return nil
	})

	body := `{"name":"John","email":"john@example.com"}`
	req := httptest.NewRequest("POST", "/json", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	_, _ = app.Test(req)
}

func TestContextBind(t *testing.T) {
	app := fiber.New()
	app.Post("/bind", func(c *fiber.Ctx) error {
		ctx := NewContext(c, &mockApplication{})
		var data struct {
			Title string `json:"title"`
		}
		err := ctx.Bind(&data)
		require.NoError(t, err)
		assert.Equal(t, "Test Title", data.Title)
		return nil
	})

	body := `{"title":"Test Title"}`
	req := httptest.NewRequest("POST", "/bind", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	_, _ = app.Test(req)
}

func TestContextGetSet(t *testing.T) {
	app := fiber.New()
	app.Get("/test", func(c *fiber.Ctx) error {
		ctx := NewContext(c, &mockApplication{})
		
		ctx.Set("user_id", 123)
		ctx.Set("name", "John")
		
		assert.Equal(t, 123, ctx.Get("user_id"))
		assert.Equal(t, "John", ctx.Get("name"))
		assert.Nil(t, ctx.Get("nonexistent"))
		return nil
	})

	req := httptest.NewRequest("GET", "/test", nil)
	_, _ = app.Test(req)
}

func TestContextAbort(t *testing.T) {
	app := fiber.New()
	app.Get("/test", func(c *fiber.Ctx) error {
		ctx := NewContext(c, &mockApplication{})
		
		assert.False(t, ctx.IsAborted())
		ctx.Abort()
		assert.True(t, ctx.IsAborted())
		return nil
	})

	req := httptest.NewRequest("GET", "/test", nil)
	_, _ = app.Test(req)
}

func TestContextAbortWithStatus(t *testing.T) {
	app := fiber.New()
	app.Get("/test", func(c *fiber.Ctx) error {
		ctx := NewContext(c, &mockApplication{})
		return ctx.AbortWithStatus(403)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	resp, _ := app.Test(req)
	assert.Equal(t, 403, resp.StatusCode)
}

func TestContextAbortWithJSON(t *testing.T) {
	app := fiber.New()
	app.Get("/test", func(c *fiber.Ctx) error {
		ctx := NewContext(c, &mockApplication{})
		return ctx.AbortWithJSON(400, fiber.Map{"error": "bad request"})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	resp, _ := app.Test(req)
	assert.Equal(t, 400, resp.StatusCode)
}

func TestContextStatus(t *testing.T) {
	app := fiber.New()
	app.Get("/test", func(c *fiber.Ctx) error {
		ctx := NewContext(c, &mockApplication{})
		ctx.Status(201)
		return ctx.String("created")
	})

	req := httptest.NewRequest("GET", "/test", nil)
	resp, _ := app.Test(req)
	assert.Equal(t, 201, resp.StatusCode)
}

func TestContextHeader(t *testing.T) {
	app := fiber.New()
	app.Get("/test", func(c *fiber.Ctx) error {
		ctx := NewContext(c, &mockApplication{})
		ctx.Header("X-Custom", "value")
		return ctx.String("ok")
	})

	req := httptest.NewRequest("GET", "/test", nil)
	resp, _ := app.Test(req)
	assert.Equal(t, "value", resp.Header.Get("X-Custom"))
}

func TestContextString(t *testing.T) {
	app := fiber.New()
	app.Get("/test", func(c *fiber.Ctx) error {
		ctx := NewContext(c, &mockApplication{})
		return ctx.String("hello world")
	})

	req := httptest.NewRequest("GET", "/test", nil)
	resp, _ := app.Test(req)
	body := make([]byte, 1024)
	n, _ := resp.Body.Read(body)
	assert.Equal(t, "hello world", string(body[:n]))
}

func TestContextJSONResponse(t *testing.T) {
	app := fiber.New()
	app.Get("/test", func(c *fiber.Ctx) error {
		ctx := NewContext(c, &mockApplication{})
		return ctx.JSONResponse(fiber.Map{"message": "success"})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	resp, _ := app.Test(req)
	assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))
}

func TestContextHTML(t *testing.T) {
	app := fiber.New()
	app.Get("/test", func(c *fiber.Ctx) error {
		ctx := NewContext(c, &mockApplication{})
		return ctx.HTML("<h1>Hello</h1>")
	})

	req := httptest.NewRequest("GET", "/test", nil)
	resp, _ := app.Test(req)
	assert.Contains(t, resp.Header.Get("Content-Type"), "text/html")
}
