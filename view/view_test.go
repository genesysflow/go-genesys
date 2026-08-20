package view_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/genesysflow/go-genesys/view"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeView(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, rel)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

func TestRenderSimpleView(t *testing.T) {
	root := t.TempDir()
	writeView(t, root, "greeting.html", "Hello, {{.name}}!")

	m := view.NewManager(view.Config{Path: root})
	out, err := m.RenderString("greeting", map[string]any{"name": "World"})
	require.NoError(t, err)
	assert.Equal(t, "Hello, World!", out)
}

func TestDotNotationAndComposition(t *testing.T) {
	root := t.TempDir()
	writeView(t, root, "layouts/app.html", "<html>{{template \"partials.nav\" .}}<main>{{.content}}</main></html>")
	writeView(t, root, "partials/nav.html", "<nav>{{.appName}}</nav>")
	writeView(t, root, "users/index.html", "{{template \"layouts.app\" .}}")

	m := view.NewManager(view.Config{Path: root})
	m.Share("appName", "Genesys")

	out, err := m.RenderString("users.index", map[string]any{"content": "user list"})
	require.NoError(t, err)
	assert.Equal(t, "<html><nav>Genesys</nav><main>user list</main></html>", out)
}

func TestHTMLEscapingAndRawHelper(t *testing.T) {
	root := t.TempDir()
	writeView(t, root, "esc.html", "{{.value}}|{{raw .trusted}}")

	m := view.NewManager(view.Config{Path: root})
	out, err := m.RenderString("esc", map[string]any{
		"value":   "<script>alert(1)</script>",
		"trusted": "<b>bold</b>",
	})
	require.NoError(t, err)
	assert.Equal(t, "&lt;script&gt;alert(1)&lt;/script&gt;|<b>bold</b>", out)
}

func TestMissingView(t *testing.T) {
	m := view.NewManager(view.Config{Path: t.TempDir()})
	_, err := m.RenderString("nope", nil)
	assert.ErrorContains(t, err, "view [nope] not found")
	assert.False(t, m.Exists("nope"))
}

func TestReloadPicksUpChanges(t *testing.T) {
	root := t.TempDir()
	writeView(t, root, "page.html", "v1")

	m := view.NewManager(view.Config{Path: root, Reload: true})
	out, err := m.RenderString("page", nil)
	require.NoError(t, err)
	assert.Equal(t, "v1", out)

	writeView(t, root, "page.html", "v2")
	out, err = m.RenderString("page", nil)
	require.NoError(t, err)
	assert.Equal(t, "v2", out)
}
