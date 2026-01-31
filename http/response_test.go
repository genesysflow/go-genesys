package http

import (
	"io"
	"net/http/httptest"
	"testing"

	"github.com/genesysflow/go-genesys/contracts"
	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
)

func TestNewResponse(t *testing.T) {
	app := fiber.New()
	app.Get("/test", func(c *fiber.Ctx) error {
		resp := NewResponse(c)
		assert.NotNil(t, resp)
		assert.Equal(t, c, resp.FiberCtx())
		assert.False(t, resp.Sent())
		return nil
	})

	req := httptest.NewRequest("GET", "/test", nil)
	_, _ = app.Test(req)
}

func TestResponseStatus(t *testing.T) {
	app := fiber.New()
	app.Get("/test", func(c *fiber.Ctx) error {
		resp := NewResponse(c)
		ret := resp.Status(201)
		assert.Equal(t, resp, ret) // Should return self for chaining
		return resp.String("created")
	})

	req := httptest.NewRequest("GET", "/test", nil)
	httpResp, _ := app.Test(req)
	assert.Equal(t, 201, httpResp.StatusCode)
}

func TestResponseHeader(t *testing.T) {
	app := fiber.New()
	app.Get("/test", func(c *fiber.Ctx) error {
		resp := NewResponse(c)
		resp.Header("X-Custom", "value")
		return resp.String("ok")
	})

	req := httptest.NewRequest("GET", "/test", nil)
	httpResp, _ := app.Test(req)
	assert.Equal(t, "value", httpResp.Header.Get("X-Custom"))
}

func TestResponseHeaders(t *testing.T) {
	app := fiber.New()
	app.Get("/test", func(c *fiber.Ctx) error {
		resp := NewResponse(c)
		resp.Headers(map[string]string{
			"X-One": "1",
			"X-Two": "2",
		})
		return resp.String("ok")
	})

	req := httptest.NewRequest("GET", "/test", nil)
	httpResp, _ := app.Test(req)
	assert.Equal(t, "1", httpResp.Header.Get("X-One"))
	assert.Equal(t, "2", httpResp.Header.Get("X-Two"))
}

func TestResponseCookie(t *testing.T) {
	app := fiber.New()
	app.Get("/test", func(c *fiber.Ctx) error {
		resp := NewResponse(c)
		resp.Cookie(&contracts.Cookie{
			Name:     "session",
			Value:    "abc123",
			Path:     "/",
			HTTPOnly: true,
		})
		return resp.String("ok")
	})

	req := httptest.NewRequest("GET", "/test", nil)
	httpResp, _ := app.Test(req)
	cookies := httpResp.Cookies()
	assert.Len(t, cookies, 1)
	assert.Equal(t, "session", cookies[0].Name)
	assert.Equal(t, "abc123", cookies[0].Value)
}

func TestResponseClearCookie(t *testing.T) {
	app := fiber.New()
	app.Get("/test", func(c *fiber.Ctx) error {
		resp := NewResponse(c)
		resp.ClearCookie("session")
		return resp.String("ok")
	})

	req := httptest.NewRequest("GET", "/test", nil)
	httpResp, _ := app.Test(req)
	// Clear cookie sets maxAge to -1 or expires in past
	cookies := httpResp.Cookies()
	if len(cookies) > 0 {
		assert.Equal(t, "session", cookies[0].Name)
	}
}

func TestResponseBody(t *testing.T) {
	app := fiber.New()
	app.Get("/test", func(c *fiber.Ctx) error {
		resp := NewResponse(c)
		return resp.Body([]byte("raw body"))
	})

	req := httptest.NewRequest("GET", "/test", nil)
	httpResp, _ := app.Test(req)
	body, _ := io.ReadAll(httpResp.Body)
	assert.Equal(t, "raw body", string(body))
}

func TestResponseString(t *testing.T) {
	app := fiber.New()
	app.Get("/test", func(c *fiber.Ctx) error {
		resp := NewResponse(c)
		return resp.String("hello world")
	})

	req := httptest.NewRequest("GET", "/test", nil)
	httpResp, _ := app.Test(req)
	body, _ := io.ReadAll(httpResp.Body)
	assert.Equal(t, "hello world", string(body))
}

func TestResponseJSON(t *testing.T) {
	app := fiber.New()
	app.Get("/test", func(c *fiber.Ctx) error {
		resp := NewResponse(c)
		return resp.JSON(fiber.Map{"message": "success"})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	httpResp, _ := app.Test(req)
	assert.Equal(t, "application/json", httpResp.Header.Get("Content-Type"))
	body, _ := io.ReadAll(httpResp.Body)
	assert.Contains(t, string(body), "success")
}

func TestResponsePrettyJSON(t *testing.T) {
	app := fiber.New()
	app.Get("/test", func(c *fiber.Ctx) error {
		resp := NewResponse(c)
		return resp.PrettyJSON(fiber.Map{"key": "value"})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	httpResp, _ := app.Test(req)
	assert.Equal(t, "application/json", httpResp.Header.Get("Content-Type"))
	body, _ := io.ReadAll(httpResp.Body)
	assert.Contains(t, string(body), "  ") // Should have indentation
}

func TestResponseXML(t *testing.T) {
	type Item struct {
		Name string `xml:"name"`
	}

	app := fiber.New()
	app.Get("/test", func(c *fiber.Ctx) error {
		resp := NewResponse(c)
		return resp.XML(Item{Name: "test"})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	httpResp, _ := app.Test(req)
	assert.Equal(t, "application/xml", httpResp.Header.Get("Content-Type"))
	body, _ := io.ReadAll(httpResp.Body)
	assert.Contains(t, string(body), "<Item>")
}

func TestResponseHTML(t *testing.T) {
	app := fiber.New()
	app.Get("/test", func(c *fiber.Ctx) error {
		resp := NewResponse(c)
		return resp.HTML("<h1>Hello</h1>")
	})

	req := httptest.NewRequest("GET", "/test", nil)
	httpResp, _ := app.Test(req)
	assert.Contains(t, httpResp.Header.Get("Content-Type"), "text/html")
	body, _ := io.ReadAll(httpResp.Body)
	assert.Equal(t, "<h1>Hello</h1>", string(body))
}

func TestResponseRedirect(t *testing.T) {
	app := fiber.New()
	app.Get("/test", func(c *fiber.Ctx) error {
		resp := NewResponse(c)
		return resp.Redirect("/new-location")
	})

	req := httptest.NewRequest("GET", "/test", nil)
	httpResp, _ := app.Test(req)
	assert.Equal(t, 302, httpResp.StatusCode)
	assert.Equal(t, "/new-location", httpResp.Header.Get("Location"))
}

func TestResponseRedirectWithStatus(t *testing.T) {
	app := fiber.New()
	app.Get("/test", func(c *fiber.Ctx) error {
		resp := NewResponse(c)
		return resp.Redirect("/permanent", 301)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	httpResp, _ := app.Test(req)
	assert.Equal(t, 301, httpResp.StatusCode)
}

func TestResponseRedirectBack(t *testing.T) {
	app := fiber.New()
	app.Get("/test", func(c *fiber.Ctx) error {
		resp := NewResponse(c)
		return resp.RedirectBack("/fallback")
	})

	// With referer
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Referer", "/previous-page")
	httpResp, _ := app.Test(req)
	assert.Equal(t, "/previous-page", httpResp.Header.Get("Location"))

	// Without referer (uses fallback)
	req = httptest.NewRequest("GET", "/test", nil)
	httpResp, _ = app.Test(req)
	assert.Equal(t, "/fallback", httpResp.Header.Get("Location"))
}

func TestResponseNoContent(t *testing.T) {
	app := fiber.New()
	app.Get("/test", func(c *fiber.Ctx) error {
		resp := NewResponse(c)
		return resp.NoContent()
	})

	req := httptest.NewRequest("GET", "/test", nil)
	httpResp, _ := app.Test(req)
	assert.Equal(t, 204, httpResp.StatusCode)
}

func TestResponseSent(t *testing.T) {
	app := fiber.New()
	app.Get("/test", func(c *fiber.Ctx) error {
		resp := NewResponse(c)
		assert.False(t, resp.Sent())
		resp.String("hello")
		assert.True(t, resp.Sent())
		return nil
	})

	req := httptest.NewRequest("GET", "/test", nil)
	_, _ = app.Test(req)
}

func TestResponseChaining(t *testing.T) {
	app := fiber.New()
	app.Get("/test", func(c *fiber.Ctx) error {
		resp := NewResponse(c)
		return resp.
			Status(201).
			Header("X-Custom", "value").
			JSON(fiber.Map{"created": true})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	httpResp, _ := app.Test(req)
	assert.Equal(t, 201, httpResp.StatusCode)
	assert.Equal(t, "value", httpResp.Header.Get("X-Custom"))
}

func TestResponseWrite(t *testing.T) {
	app := fiber.New()
	app.Get("/test", func(c *fiber.Ctx) error {
		resp := NewResponse(c)
		n, err := resp.Write([]byte("written"))
		assert.NoError(t, err)
		assert.Equal(t, 7, n)
		return nil
	})

	req := httptest.NewRequest("GET", "/test", nil)
	httpResp, _ := app.Test(req)
	body, _ := io.ReadAll(httpResp.Body)
	assert.Equal(t, "written", string(body))
}
