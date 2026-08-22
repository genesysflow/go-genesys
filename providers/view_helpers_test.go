package providers_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/genesysflow/go-genesys/container"
	"github.com/genesysflow/go-genesys/foundation"
	genhttp "github.com/genesysflow/go-genesys/http"
	"github.com/genesysflow/go-genesys/providers"
	"github.com/genesysflow/go-genesys/view"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The view helpers are only useful once they are wired to the router,
// the config, and the translator - which is the provider's job, not the
// application's.
func TestViewProviderWiresHelpers(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(
		filepath.Join(root, "page.html"),
		[]byte(`{{route "users.show" (dict "user" 42)}}|{{config "app.name"}}|{{trans "messages.hi"}}`),
		0o600,
	))

	app := foundation.New()
	require.NoError(t, app.Register(&providers.ViewServiceProvider{Config: &view.Config{Path: root}}))
	require.NoError(t, app.Register(&providers.RouteServiceProvider{
		Routes: func(r *genhttp.Router) {
			r.GET("/users/:user", func(ctx *genhttp.Context) error { return nil }).Name("users.show")
		},
	}))
	require.NoError(t, app.Boot())

	app.GetConfig().Set("app.name", "Genesys")

	manager := container.MustResolve[*view.Manager](app)
	out, err := manager.RenderString("page", nil)
	require.NoError(t, err)

	assert.Equal(t, "/users/42|Genesys|messages.hi", out)
}

// The base URL for url()/asset() comes from app.url.
func TestViewProviderWiresBaseURL(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(
		filepath.Join(root, "page.html"),
		[]byte(`{{asset "css/app.css"}}`),
		0o600,
	))

	app := foundation.New()
	app.GetConfig().Set("app.url", "https://example.com")
	require.NoError(t, app.Register(&providers.ViewServiceProvider{Config: &view.Config{Path: root}}))
	require.NoError(t, app.Boot())

	manager := container.MustResolve[*view.Manager](app)
	out, err := manager.RenderString("page", nil)
	require.NoError(t, err)
	assert.Equal(t, "https://example.com/css/app.css", out)
}
