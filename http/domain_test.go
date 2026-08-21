package http_test

import (
	"io"
	"net/http/httptest"
	"testing"

	genhttp "github.com/genesysflow/go-genesys/http"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSubdomainRouting(t *testing.T) {
	k := bootKernel(t)

	k.Router().Domain("{account}.example.com", func(r *genhttp.Router) {
		r.GET("/dashboard", func(ctx *genhttp.Context) error {
			return ctx.String("tenant:" + genhttp.DomainParam(ctx, "account"))
		})
	})
	k.Router().Domain("admin.example.com", func(r *genhttp.Router) {
		r.GET("/panel", func(ctx *genhttp.Context) error {
			return ctx.String("admin panel")
		})
	})
	// Apex fallback for the same path as the tenant route.
	k.GET("/dashboard", func(ctx *genhttp.Context) error {
		return ctx.String("apex dashboard")
	})

	send := func(host, path string) (int, string) {
		req := httptest.NewRequest("GET", path, nil)
		req.Host = host
		resp, err := k.Fiber().Test(req, -1)
		require.NoError(t, err)
		content, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(content)
	}

	// Wildcard subdomain captures the tenant.
	status, content := send("acme.example.com", "/dashboard")
	assert.Equal(t, 200, status)
	assert.Equal(t, "tenant:acme", content)

	status, content = send("globex.example.com", "/dashboard")
	assert.Equal(t, 200, status)
	assert.Equal(t, "tenant:globex", content)

	// Apex host falls through the domain group to the apex route.
	status, content = send("example.com", "/dashboard")
	assert.Equal(t, 200, status)
	assert.Equal(t, "apex dashboard", content)

	// Fixed-subdomain routes only answer on their host.
	status, content = send("admin.example.com", "/panel")
	assert.Equal(t, 200, status)
	assert.Equal(t, "admin panel", content)
	status, _ = send("other.example.com", "/panel")
	assert.Equal(t, 404, status)

	// Ports are stripped before matching.
	status, content = send("acme.example.com:8080", "/dashboard")
	assert.Equal(t, 200, status)
	assert.Equal(t, "tenant:acme", content)

	// Deeper hosts don't match the wildcard pattern; they fall through
	// to the host-agnostic apex route like any other non-matching host.
	status, content = send("a.b.example.com", "/dashboard")
	assert.Equal(t, 200, status)
	assert.Equal(t, "apex dashboard", content)
}
