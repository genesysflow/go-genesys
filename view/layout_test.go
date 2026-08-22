package view_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/genesysflow/go-genesys/view"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// layoutViews writes a views directory with a layout and two pages.
func layoutViews(t *testing.T) string {
	t.Helper()

	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "layouts"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "posts"), 0o755))

	write := func(path, content string) {
		require.NoError(t, os.WriteFile(filepath.Join(root, filepath.FromSlash(path)), []byte(content), 0o600))
	}

	write("layouts/app.html", `<html><body><h1>{{.title}}</h1><main>{{.content}}</main></body></html>`)
	write("layouts/bare.html", `<main>{{.content}}</main>`)
	write("posts/index.html", `<ul>{{range .posts}}<li>{{.}}</li>{{end}}</ul>`)
	write("posts/partial.html", `<span>{{.name}}</span>`)

	return root
}

// A view renders inside the application's layout, the way every page of
// a site shares its chrome.
func TestRenderInsideTheDefaultLayout(t *testing.T) {
	manager := view.NewManager(view.Config{Path: layoutViews(t)})
	manager.SetLayout("layouts.app")

	out, err := manager.RenderString("posts.index", map[string]any{
		"title": "Posts",
		"posts": []string{"one", "two"},
	})
	require.NoError(t, err)

	assert.Contains(t, out, "<html>")
	assert.Contains(t, out, "<h1>Posts</h1>")
	assert.Contains(t, out, "<li>one</li>")

	// The page's own markup is not escaped on its way into the layout.
	assert.NotContains(t, out, "&lt;li&gt;")
}

// A page may name its own layout, for the one that does not share the
// site's chrome.
func TestRenderInsideANamedLayout(t *testing.T) {
	manager := view.NewManager(view.Config{Path: layoutViews(t)})
	manager.SetLayout("layouts.app")

	out, err := manager.RenderStringIn("layouts.bare", "posts.index", map[string]any{
		"posts": []string{"one"},
	})
	require.NoError(t, err)

	assert.Contains(t, out, "<li>one</li>")
	assert.NotContains(t, out, "<html>")
}

// A partial rendered on its own - an ajax fragment, an email body - opts
// out of the layout entirely.
func TestRenderWithoutALayout(t *testing.T) {
	manager := view.NewManager(view.Config{Path: layoutViews(t)})
	manager.SetLayout("layouts.app")

	out, err := manager.RenderStringIn("", "posts.partial", map[string]any{"name": "Ada"})
	require.NoError(t, err)

	assert.Equal(t, "<span>Ada</span>", strings.TrimSpace(out))
}

// Without a layout configured, a view renders exactly as it did before.
func TestRenderWithNoLayoutConfigured(t *testing.T) {
	manager := view.NewManager(view.Config{Path: layoutViews(t)})

	out, err := manager.RenderString("posts.partial", map[string]any{"name": "Ada"})
	require.NoError(t, err)
	assert.Equal(t, "<span>Ada</span>", strings.TrimSpace(out))
}

// A layout is a view like any other, so rendering one directly must not
// wrap it in itself.
func TestALayoutIsNotWrappedInItself(t *testing.T) {
	manager := view.NewManager(view.Config{Path: layoutViews(t)})
	manager.SetLayout("layouts.app")

	out, err := manager.RenderString("layouts.app", map[string]any{"title": "Direct"})
	require.NoError(t, err)
	assert.Equal(t, 1, strings.Count(out, "<html>"))
}

// A layout that does not exist is an error naming it, not a blank page.
func TestAMissingLayoutIsAnError(t *testing.T) {
	manager := view.NewManager(view.Config{Path: layoutViews(t)})

	_, err := manager.RenderStringIn("layouts.nope", "posts.partial", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "layouts.nope")
}
