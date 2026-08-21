package http_test

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/genesysflow/go-genesys/contracts"
	genhttp "github.com/genesysflow/go-genesys/http"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContextRequestAccessors(t *testing.T) {
	k := bootKernel(t)
	k.POST("/users/:id/posts", func(ctx *genhttp.Context) error {
		assert.Equal(t, "42", ctx.Param("id"))
		assert.Equal(t, 42, ctx.ParamInt("id"))
		assert.Equal(t, "fallback", ctx.Param("missing", "fallback"))
		assert.Equal(t, 7, ctx.ParamInt("missing", 7))

		assert.Equal(t, "golang", ctx.Query("q"))
		assert.Equal(t, 3, ctx.QueryInt("page"))
		assert.Equal(t, "def", ctx.Query("nope", "def"))

		assert.Equal(t, "POST", ctx.Method())
		assert.Equal(t, "/users/42/posts", ctx.Path())
		assert.NotEmpty(t, ctx.IP())

		var body map[string]any
		require.NoError(t, ctx.JSON(&body))
		assert.Equal(t, "hello", body["title"])

		// Per-request store
		ctx.Set("k", "v")
		assert.Equal(t, "v", ctx.Get("k"))
		assert.Nil(t, ctx.Get("missing"))

		return ctx.NoContent()
	})

	req := httptest.NewRequest("POST", "/users/42/posts?q=golang&page=3",
		strings.NewReader(`{"title":"hello"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := k.Fiber().Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, 204, resp.StatusCode)
}

func TestContextResponseHelpers(t *testing.T) {
	k := bootKernel(t)

	k.GET("/string", func(ctx *genhttp.Context) error { return ctx.String("plain") })
	k.GET("/html", func(ctx *genhttp.Context) error { return ctx.HTML("<b>hi</b>") })
	k.GET("/bytes", func(ctx *genhttp.Context) error { return ctx.Send([]byte{1, 2}) })
	k.GET("/created", func(ctx *genhttp.Context) error { return ctx.Created(map[string]any{"id": 1}) })
	k.GET("/accepted", func(ctx *genhttp.Context) error { return ctx.Accepted() })
	k.GET("/status", func(ctx *genhttp.Context) error {
		return ctx.Status(418).Header("X-Tea", "pot").(*genhttp.Context).String("teapot")
	})
	k.GET("/bad", func(ctx *genhttp.Context) error { return ctx.BadRequest("nope") })
	k.GET("/unauthorized", func(ctx *genhttp.Context) error { return ctx.Unauthorized() })
	k.GET("/forbidden", func(ctx *genhttp.Context) error { return ctx.Forbidden() })
	k.GET("/notfound", func(ctx *genhttp.Context) error { return ctx.NotFound() })
	k.GET("/ise", func(ctx *genhttp.Context) error { return ctx.InternalServerError() })
	k.GET("/redirect", func(ctx *genhttp.Context) error { return ctx.Redirect("/target") })
	k.GET("/redirect301", func(ctx *genhttp.Context) error { return ctx.Redirect("/target", 301) })

	expect := func(path string, status int, bodyContains string) {
		t.Helper()
		resp, err := k.Fiber().Test(httptest.NewRequest("GET", path, nil), -1)
		require.NoError(t, err)
		assert.Equal(t, status, resp.StatusCode, path)
		if bodyContains != "" {
			body, _ := io.ReadAll(resp.Body)
			assert.Contains(t, string(body), bodyContains, path)
		}
	}

	expect("/string", 200, "plain")
	expect("/html", 200, "<b>hi</b>")
	expect("/bytes", 200, "")
	expect("/created", 201, `"id":1`)
	expect("/accepted", 202, "")
	expect("/status", 418, "teapot")
	expect("/bad", 400, "nope")
	expect("/unauthorized", 401, "Unauthorized")
	expect("/forbidden", 403, "Forbidden")
	expect("/notfound", 404, "Not Found")
	expect("/ise", 500, "Internal Server Error")

	resp, err := k.Fiber().Test(httptest.NewRequest("GET", "/redirect", nil), -1)
	require.NoError(t, err)
	assert.Equal(t, 302, resp.StatusCode)
	assert.Equal(t, "/target", resp.Header.Get("Location"))

	resp, err = k.Fiber().Test(httptest.NewRequest("GET", "/redirect301", nil), -1)
	require.NoError(t, err)
	assert.Equal(t, 301, resp.StatusCode)
}

func TestContextContentNegotiationHelpers(t *testing.T) {
	k := bootKernel(t)
	k.GET("/negotiate", func(ctx *genhttp.Context) error {
		return ctx.JSONResponse(map[string]any{
			"ajax": ctx.IsAjax(),
			"json": ctx.IsJSON(),
		})
	})

	req := httptest.NewRequest("GET", "/negotiate", nil)
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	req.Header.Set("Accept", "application/json")
	resp, err := k.Fiber().Test(req, -1)
	require.NoError(t, err)
	var out map[string]bool
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
	assert.True(t, out["ajax"])
	assert.True(t, out["json"])
}

func TestContextCookies(t *testing.T) {
	k := bootKernel(t)
	k.GET("/set-cookie", func(ctx *genhttp.Context) error {
		ctx.Cookie(&contracts.Cookie{Name: "prefs", Value: "dark", Path: "/", HTTPOnly: true})
		return ctx.NoContent()
	})
	k.GET("/clear-cookie", func(ctx *genhttp.Context) error {
		ctx.ClearCookie("prefs")
		return ctx.NoContent()
	})

	resp, err := k.Fiber().Test(httptest.NewRequest("GET", "/set-cookie", nil), -1)
	require.NoError(t, err)
	cookies := resp.Cookies()
	require.NotEmpty(t, cookies)
	assert.Equal(t, "prefs", cookies[0].Name)
	assert.Equal(t, "dark", cookies[0].Value)
	assert.True(t, cookies[0].HttpOnly)

	resp, err = k.Fiber().Test(httptest.NewRequest("GET", "/clear-cookie", nil), -1)
	require.NoError(t, err)
	require.NotEmpty(t, resp.Cookies())
	assert.Empty(t, resp.Cookies()[0].Value, "clearing sets an empty expired cookie")
}
