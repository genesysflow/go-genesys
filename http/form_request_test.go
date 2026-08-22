package http_test

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/genesysflow/go-genesys/foundation"
	genhttp "github.com/genesysflow/go-genesys/http"
	"github.com/genesysflow/go-genesys/providers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type storeUserRequest struct {
	Name  string `json:"name" validate:"required,min=3"`
	Email string `json:"email" validate:"required,email"`
}

func formKernel(t *testing.T) *genhttp.Kernel {
	t.Helper()
	app := foundation.New()
	require.NoError(t, app.Register(&providers.AppServiceProvider{}))
	require.NoError(t, app.Register(&providers.ValidationServiceProvider{}))
	require.NoError(t, app.Boot())

	k := genhttp.NewKernel(app, genhttp.KernelConfig{DisableStartupMessage: true})
	k.POST("/users", func(ctx *genhttp.Context) error {
		req, err := genhttp.ValidateRequest[storeUserRequest](ctx)
		if err != nil {
			return err
		}
		return ctx.Created(map[string]any{"name": req.Name})
	})
	k.GET("/search", func(ctx *genhttp.Context) error {
		type searchRequest struct {
			Q string `query:"q" json:"q" validate:"required"`
		}
		req, err := genhttp.ValidateRequest[searchRequest](ctx)
		if err != nil {
			return err
		}
		return ctx.JSONResponse(map[string]any{"q": req.Q})
	})
	return k
}

func TestValidateRequestSuccess(t *testing.T) {
	k := formKernel(t)

	req := httptest.NewRequest("POST", "/users", strings.NewReader(`{"name":"Alice","email":"alice@example.com"}`))
	req.Header.Set("Content-Type", "application/json")

	resp, err := k.Fiber().Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, 201, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	assert.Contains(t, string(body), "Alice")
}

func TestValidateRequestFailureShape(t *testing.T) {
	k := formKernel(t)

	req := httptest.NewRequest("POST", "/users", strings.NewReader(`{"name":"Al","email":"not-an-email"}`))
	req.Header.Set("Content-Type", "application/json")

	resp, err := k.Fiber().Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, 422, resp.StatusCode)

	var payload struct {
		Message string              `json:"message"`
		Errors  map[string][]string `json:"errors"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&payload))
	assert.Equal(t, "The given data was invalid.", payload.Message)
	assert.NotEmpty(t, payload.Errors["name"])
	assert.NotEmpty(t, payload.Errors["email"])
}

func TestValidateRequestBindsQueryOnGet(t *testing.T) {
	k := formKernel(t)

	resp, err := k.Fiber().Test(httptest.NewRequest("GET", "/search?q=golang", nil), -1)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	assert.Contains(t, string(body), "golang")

	resp, err = k.Fiber().Test(httptest.NewRequest("GET", "/search", nil), -1)
	require.NoError(t, err)
	assert.Equal(t, 422, resp.StatusCode)
}

func TestResourceHelpers(t *testing.T) {
	app := foundation.New()
	require.NoError(t, app.Boot())
	k := genhttp.NewKernel(app, genhttp.KernelConfig{DisableStartupMessage: true})

	k.GET("/one", func(ctx *genhttp.Context) error {
		return ctx.Resource(map[string]any{"id": 1})
	})
	k.GET("/page", func(ctx *genhttp.Context) error {
		return ctx.Paginated(struct {
			Data  []int `json:"data"`
			Total int   `json:"total"`
			Page  int   `json:"current_page"`
		}{Data: []int{1, 2}, Total: 10, Page: 1})
	})

	resp, err := k.Fiber().Test(httptest.NewRequest("GET", "/one", nil), -1)
	require.NoError(t, err)
	var one map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&one))
	assert.Equal(t, map[string]any{"id": float64(1)}, one["data"])

	resp, err = k.Fiber().Test(httptest.NewRequest("GET", "/page", nil), -1)
	require.NoError(t, err)
	var page map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&page))
	assert.Equal(t, []any{float64(1), float64(2)}, page["data"])
	meta := page["meta"].(map[string]any)
	assert.Equal(t, float64(10), meta["total"])
	assert.Equal(t, float64(1), meta["current_page"])
}

// A resource that was just created is enveloped like every other, so a
// client reads data.* whether it just wrote the record or fetched it.
func TestCreatedResourceIsEnveloped(t *testing.T) {
	app := foundation.New()
	require.NoError(t, app.Boot())
	k := genhttp.NewKernel(app, genhttp.KernelConfig{DisableStartupMessage: true})

	k.POST("/posts", func(ctx *genhttp.Context) error {
		return ctx.CreatedResource(map[string]any{"id": 7, "slug": "ada"})
	})

	resp, err := k.Fiber().Test(httptest.NewRequest("POST", "/posts", nil), -1)
	require.NoError(t, err)
	require.Equal(t, 201, resp.StatusCode)

	var created map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&created))
	data, ok := created["data"].(map[string]any)
	require.True(t, ok, "a created resource should carry a data envelope")
	assert.Equal(t, "ada", data["slug"])
}
