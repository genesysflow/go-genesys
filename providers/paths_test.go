package providers_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/genesysflow/go-genesys/container"
	"github.com/genesysflow/go-genesys/foundation"
	"github.com/genesysflow/go-genesys/lang"
	"github.com/genesysflow/go-genesys/providers"
	"github.com/genesysflow/go-genesys/view"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Configured paths are written relative to the application, not to
// whatever directory the process happens to start in - a test binary
// runs in its own package directory, and a service runs wherever it was
// launched.
func TestViewPathResolvesAgainstTheBasePath(t *testing.T) {
	root := t.TempDir()
	views := filepath.Join(root, "resources", "views")
	require.NoError(t, os.MkdirAll(views, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(views, "page.html"), []byte("hello"), 0o600))

	app := foundation.New(root)
	app.GetConfig().Set("view.path", "resources/views")
	require.NoError(t, app.Register(&providers.ViewServiceProvider{}))
	require.NoError(t, app.Boot())

	manager := container.MustResolve[*view.Manager](app)
	out, err := manager.RenderString("page", nil)
	require.NoError(t, err)
	assert.Equal(t, "hello", out)
}

// An absolute configured path is left alone.
func TestViewPathKeepsAnAbsolutePath(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "page.html"), []byte("hi"), 0o600))

	app := foundation.New(t.TempDir())
	app.GetConfig().Set("view.path", root)
	require.NoError(t, app.Register(&providers.ViewServiceProvider{}))
	require.NoError(t, app.Boot())

	manager := container.MustResolve[*view.Manager](app)
	out, err := manager.RenderString("page", nil)
	require.NoError(t, err)
	assert.Equal(t, "hi", out)
}

// Translations are addressed the same way.
func TestLangPathResolvesAgainstTheBasePath(t *testing.T) {
	root := t.TempDir()
	langDir := filepath.Join(root, "lang", "en")
	require.NoError(t, os.MkdirAll(langDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(langDir, "messages.json"), []byte(`{"hi":"Hello"}`), 0o600))

	app := foundation.New(root)
	app.GetConfig().Set("lang.path", "lang")
	app.GetConfig().Set("app.locale", "en")
	require.NoError(t, app.Register(&providers.LangServiceProvider{}))
	require.NoError(t, app.Boot())

	translator := container.MustResolve[*lang.Translator](app)
	assert.Equal(t, "Hello", translator.Trans("messages.hi"))
}
