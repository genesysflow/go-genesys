package http_test

import (
	"net/http/httptest"
	"testing"

	"github.com/genesysflow/go-genesys/foundation"
	genhttp "github.com/genesysflow/go-genesys/http"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHealthEndpoint(t *testing.T) {
	app := foundation.New(t.TempDir())
	require.NoError(t, app.Boot())
	kernel := genhttp.NewKernel(app, genhttp.KernelConfig{DisableStartupMessage: true})
	kernel.Router().Health()
	kernel.Router().Health("/healthz")

	for _, path := range []string{"/up", "/healthz"} {
		resp, err := kernel.Fiber().Test(httptest.NewRequest("GET", path, nil), -1)
		require.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode, path)
	}
}
