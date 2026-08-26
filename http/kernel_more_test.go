package http_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	genhttp "github.com/genesysflow/go-genesys/http"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- verb registration ---

// PUT, PATCH and DELETE reach the router the same way GET and POST do.
func TestKernelWriteVerbs(t *testing.T) {
	k := bootKernel(t)

	k.PUT("/posts/:id", func(ctx *genhttp.Context) error {
		return ctx.String("put:" + ctx.Param("id"))
	}).Name("posts.update")
	k.PATCH("/posts/:id", func(ctx *genhttp.Context) error {
		return ctx.String("patch:" + ctx.Param("id"))
	}).Name("posts.patch")
	k.DELETE("/posts/:id", func(ctx *genhttp.Context) error {
		return ctx.String("delete:" + ctx.Param("id"))
	}).Name("posts.destroy")

	tc := genhttp.NewTestCase(t, k)
	tc.Put("/posts/7").AssertOK().AssertSee("put:7")
	tc.Patch("/posts/7").AssertOK().AssertSee("patch:7")
	tc.Delete("/posts/7").AssertOK().AssertSee("delete:7")

	// Each returned a *Route, so it can be named and resolved.
	assert.Equal(t, "/posts/7", k.Router().URL("posts.update", map[string]any{"id": 7}))
	assert.Equal(t, "PATCH", k.Router().NamedRoute("posts.patch").GetMethod())
	assert.Equal(t, "DELETE", k.Router().NamedRoute("posts.destroy").GetMethod())
}

// A verb with no route registered is a 405, not a 404, which is what
// tells a client the resource exists but the method does not.
func TestKernelWriteVerbsAreMethodScoped(t *testing.T) {
	k := bootKernel(t)
	k.PUT("/posts/:id", func(ctx *genhttp.Context) error { return ctx.String("put") })

	genhttp.NewTestCase(t, k).Delete("/posts/7").AssertStatus(405)
}

// --- Static ---

func TestKernelStatic(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "app.css"), []byte("body{color:red}"), 0o600))

	k := bootKernel(t)
	k.Static("/assets", dir)

	genhttp.NewTestCase(t, k).
		Get("/assets/app.css").
		AssertOK().
		AssertSee("body{color:red}")
}

func TestKernelStaticMissingFile(t *testing.T) {
	k := bootKernel(t)
	k.Static("/assets", t.TempDir())

	genhttp.NewTestCase(t, k).Get("/assets/absent.css").AssertNotFound()
}

// --- Kernel.Test ---

// Test is the toolkit's entry point for code that wants the response
// rather than the assertions.
func TestKernelTest(t *testing.T) {
	k := bootKernel(t)
	k.POST("/users", func(ctx *genhttp.Context) error {
		var body map[string]any
		require.NoError(t, ctx.JSON(&body))
		return ctx.Created(map[string]any{"name": body["name"]})
	})

	resp, err := k.Test(genhttp.Post("/users").WithJSON(map[string]string{"name": "Ada"}))
	require.NoError(t, err)
	require.NotNil(t, resp)

	assert.True(t, resp.IsCreated())
	assert.True(t, resp.IsSuccess())

	var decoded map[string]any
	require.NoError(t, resp.JSON(&decoded))
	assert.Equal(t, "Ada", decoded["name"])
}

// A route that does not exist still produces a response, so the caller
// can assert on the 404 rather than on an error.
func TestKernelTestUnknownRoute(t *testing.T) {
	resp, err := bootKernel(t).Test(genhttp.Get("/absent"))

	require.NoError(t, err)
	assert.True(t, resp.IsNotFound())
	assert.True(t, resp.IsClientError())
}

// Test carries the request's headers and cookies through.
func TestKernelTestSendsHeadersAndCookies(t *testing.T) {
	k := bootKernel(t)
	k.GET("/echo", func(ctx *genhttp.Context) error {
		return ctx.JSONResponse(map[string]any{
			"authorization": ctx.Request().Header("Authorization"),
			"session":       ctx.Request().Cookie("session"),
		})
	})

	resp, err := k.Test(genhttp.Get("/echo").
		WithBearerToken("tok-1").
		WithCookie("session", "abc123"))
	require.NoError(t, err)

	var decoded map[string]any
	require.NoError(t, resp.JSON(&decoded))
	assert.Equal(t, "Bearer tok-1", decoded["authorization"])
	assert.Equal(t, "abc123", decoded["session"])
}

// --- lifecycle ---

// Shutting down a kernel that never listened is a no-op rather than an
// error, so a test or a short-lived command can call it unconditionally.
func TestKernelShutdownWithContextWhenNotRunning(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	assert.NoError(t, bootKernel(t).ShutdownWithContext(ctx))
}

// An already-cancelled context is honoured the same way.
func TestKernelShutdownWithCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	assert.NoError(t, bootKernel(t).ShutdownWithContext(ctx))
}

// Run surfaces the listen failure instead of blocking forever.
func TestKernelRunReportsListenFailure(t *testing.T) {
	err := bootKernel(t).Run("127.0.0.1:99999")
	require.Error(t, err)
}

// RunTLS refuses to start without a usable key pair.
func TestKernelRunTLSReportsCertificateFailure(t *testing.T) {
	k := bootKernel(t)

	assert.Error(t, k.RunTLS("127.0.0.1:0", "", ""), "an empty cert path is rejected up front")

	err := k.RunTLS("127.0.0.1:0",
		filepath.Join(t.TempDir(), "absent.pem"),
		filepath.Join(t.TempDir(), "absent.key"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "tls")
}

// RunWithGracefulShutdown returns the listen error rather than waiting
// for a signal that will never arrive.
func TestKernelRunWithGracefulShutdownReportsListenFailure(t *testing.T) {
	done := make(chan error, 1)
	go func() {
		done <- bootKernel(t).RunWithGracefulShutdown("127.0.0.1:99999", time.Second)
	}()

	select {
	case err := <-done:
		require.Error(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("RunWithGracefulShutdown did not return the listen error")
	}
}
