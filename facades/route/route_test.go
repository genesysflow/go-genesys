package route_test

import (
	"testing"

	"github.com/genesysflow/go-genesys/facades/route"
	"github.com/genesysflow/go-genesys/foundation"
	genhttp "github.com/genesysflow/go-genesys/http"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFacadeURLGeneration(t *testing.T) {
	app := foundation.New()
	require.NoError(t, app.Boot())
	kernel := genhttp.NewKernel(app, genhttp.KernelConfig{DisableStartupMessage: true})
	kernel.GET("/users/:id", func(ctx *genhttp.Context) error { return nil }).Name("users.show")

	route.SetInstance(kernel.Router())
	t.Cleanup(func() { route.SetInstance(nil) })

	assert.Equal(t, "/users/42", route.URL("users.show", map[string]any{"id": 42}))
	assert.True(t, route.Has("users.show"))
	assert.False(t, route.Has("users.missing"))
	assert.NotNil(t, route.GetInstance())
}

func TestFacadePanicsWithoutInstance(t *testing.T) {
	route.SetInstance(nil)
	assert.Panics(t, func() { route.URL("x") })
}
