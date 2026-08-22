package view_test

import (
	"html/template"
	"os"
	"path/filepath"
	"testing"

	"github.com/genesysflow/go-genesys/view"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// xssViews writes a views directory for the escaping tests.
func xssViews(t *testing.T) string {
	t.Helper()

	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "layouts"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "components"), 0o755))

	write := func(path, content string) {
		require.NoError(t, os.WriteFile(filepath.Join(root, filepath.FromSlash(path)), []byte(content), 0o600))
	}

	write("layouts/app.html", `<html><title>{{.title}}</title><main>{{.content}}</main></html>`)
	write("page.html", `<p>{{.name}}</p><a href="{{.link}}">go</a>`)
	write("components/card.html", `<span>{{.label}}</span>`)
	write("uses_component.html", `{{component "card" (dict "label" .label)}}`)

	return root
}

// The page's own values are escaped, as html/template does by default.
func TestPageValuesAreEscaped(t *testing.T) {
	manager := view.NewManager(view.Config{Path: xssViews(t)})

	out, err := manager.RenderString("page", map[string]any{
		"name": `<script>alert(1)</script>`,
		"link": "https://example.com",
	})
	require.NoError(t, err)

	assert.NotContains(t, out, "<script>alert(1)</script>")
	assert.Contains(t, out, "&lt;script&gt;")
}

// A javascript: URL in an href is neutralised by the template engine's
// URL context, not passed through.
func TestJavascriptURLsAreNeutralised(t *testing.T) {
	manager := view.NewManager(view.Config{Path: xssViews(t)})

	out, err := manager.RenderString("page", map[string]any{
		"name": "Ada",
		"link": "javascript:alert(1)",
	})
	require.NoError(t, err)

	assert.NotContains(t, out, `href="javascript:alert(1)"`)
}

// The layout receives the page as already-rendered HTML. That must not
// become a hole: a value inside the page is still escaped, and a value
// the layout renders itself is escaped by the layout.
func TestLayoutDoesNotUnescapeThePage(t *testing.T) {
	manager := view.NewManager(view.Config{Path: xssViews(t)})
	manager.SetLayout("layouts.app")

	out, err := manager.RenderString("page", map[string]any{
		"title": `<script>alert("title")</script>`,
		"name":  `<script>alert("body")</script>`,
		"link":  "https://example.com",
	})
	require.NoError(t, err)

	assert.NotContains(t, out, `<script>alert("body")</script>`,
		"a value rendered by the page must stay escaped through the layout")
	assert.NotContains(t, out, `<script>alert("title")</script>`,
		"a value rendered by the layout must be escaped too")

	// The page's own markup does survive, which is the point of the
	// layout - it is the data inside it that is escaped.
	assert.Contains(t, out, "<main><p>")
}

// A component renders its data escaped too.
func TestComponentValuesAreEscaped(t *testing.T) {
	manager := view.NewManager(view.Config{Path: xssViews(t)})

	out, err := manager.RenderString("uses_component", map[string]any{
		"label": `<img src=x onerror=alert(1)>`,
	})
	require.NoError(t, err)

	assert.NotContains(t, out, "<img src=x")
	assert.Contains(t, out, "&lt;img")
}

// A component's slot is often filled from a handler, which means from
// user data. A plain string is data and is escaped; markup says so by
// arriving as template.HTML, which is what `raw` produces.
func TestSlotEscapesAStringButNotMarkup(t *testing.T) {
	root := xssViews(t)
	require.NoError(t, os.WriteFile(filepath.Join(root, "components", "alert.html"),
		[]byte(`<div class="alert">{{slot .}}</div>`), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(root, "uses_slot.html"),
		[]byte(`{{component "alert" (dict "slot" .body)}}`), 0o600))

	manager := view.NewManager(view.Config{Path: root})

	// A string from the request is data.
	out, err := manager.RenderString("uses_slot", map[string]any{
		"body": `<script>alert(1)</script>`,
	})
	require.NoError(t, err)
	assert.NotContains(t, out, "<script>alert(1)</script>")
	assert.Contains(t, out, "&lt;script&gt;")

	// Markup the application vouched for still renders.
	out, err = manager.RenderString("uses_slot", map[string]any{
		"body": template.HTML(`<strong>Careful</strong>`),
	})
	require.NoError(t, err)
	assert.Contains(t, out, "<strong>Careful</strong>")
}
