package http_test

import (
	"testing"

	genhttp "github.com/genesysflow/go-genesys/http"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNamedRouteURLGeneration(t *testing.T) {
	k := bootKernel(t)
	router := k.Router()

	k.GET("/users/:id", func(ctx *genhttp.Context) error { return nil }).Name("users.show")
	k.GET("/teams/:team/members/:id", func(ctx *genhttp.Context) error { return nil }).Name("members.show")
	k.GET("/files/:idx", func(ctx *genhttp.Context) error { return nil }).Name("files.show")

	assert.Equal(t, "/users/42", router.URL("users.show", map[string]any{"id": 42}))
	assert.Equal(t, "/teams/7/members/9", router.URL("members.show", map[string]any{"team": 7, "id": 9}))

	// :id must not corrupt :idx.
	assert.Equal(t, "/files/:idx", router.URL("files.show", map[string]any{"id": 1}))
	assert.Equal(t, "/files/3", router.URL("files.show", map[string]any{"idx": 3}))

	// Values are path-escaped.
	assert.Equal(t, "/users/a%2Fb", router.URL("users.show", map[string]any{"id": "a/b"}))

	// Unknown routes return empty.
	assert.Equal(t, "", router.URL("missing.route"))

	// Named routes registered inside groups are reachable from the root.
	k.Group("/api", func(r *genhttp.Router) {
		r.GET("/things/:id", func(ctx *genhttp.Context) error { return nil }).Name("api.things.show")
	})
	require.NotNil(t, router.NamedRoute("api.things.show"))
	assert.Equal(t, "/api/things/5", router.URL("api.things.show", map[string]any{"id": 5}))
}
