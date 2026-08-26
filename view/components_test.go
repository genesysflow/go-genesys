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

func writeComponent(t *testing.T, root, name, content string) {
	t.Helper()
	path := filepath.Join(root, "components", name)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

func TestComponentsRenderWithData(t *testing.T) {
	root := t.TempDir()
	writeComponent(t, root, "alert.html",
		`<div class="alert alert-{{.type}}">{{.message}}</div>`)
	writeView(t, root, "page.html",
		`<h1>Page</h1>{{component "alert" (dict "type" "error" "message" "Something broke")}}`)

	m := view.NewManager(view.Config{Path: root})
	out, err := m.RenderString("page", nil)
	require.NoError(t, err)
	assert.Equal(t, `<h1>Page</h1><div class="alert alert-error">Something broke</div>`, out)
}

func TestComponentSlots(t *testing.T) {
	root := t.TempDir()
	writeComponent(t, root, "card.html",
		`<div class="card"><h2>{{.title}}</h2><div class="body">{{slot .}}</div></div>`)
	// Markup in a slot is markup the template author wrote, so it says
	// so with raw; a bare string is data and would be escaped.
	writeView(t, root, "page.html",
		`{{component "card" (dict "title" "Hello" "slot" (raw "<p>Inner content</p>"))}}`)

	m := view.NewManager(view.Config{Path: root})
	out, err := m.RenderString("page", nil)
	require.NoError(t, err)
	assert.Equal(t,
		`<div class="card"><h2>Hello</h2><div class="body"><p>Inner content</p></div></div>`, out)
}

func TestNestedComponents(t *testing.T) {
	root := t.TempDir()
	writeComponent(t, root, "badge.html", `<span class="badge">{{.label}}</span>`)
	writeComponent(t, root, "header.html",
		`<header>{{.name}} {{component "badge" (dict "label" .role)}}</header>`)
	writeView(t, root, "page.html",
		`{{component "header" (dict "name" "Ada" "role" "admin")}}`)

	m := view.NewManager(view.Config{Path: root})
	out, err := m.RenderString("page", nil)
	require.NoError(t, err)
	assert.Equal(t, `<header>Ada <span class="badge">admin</span></header>`, out)
}

func TestComponentDataIsEscaped(t *testing.T) {
	root := t.TempDir()
	writeComponent(t, root, "alert.html", `<div>{{.message}}</div>`)
	writeView(t, root, "page.html",
		`{{component "alert" (dict "message" .userInput)}}`)

	m := view.NewManager(view.Config{Path: root})
	out, err := m.RenderString("page", map[string]any{"userInput": `<script>alert(1)</script>`})
	require.NoError(t, err)
	assert.NotContains(t, out, "<script>", "regular data stays escaped inside components")
}

func TestMissingComponentErrors(t *testing.T) {
	root := t.TempDir()
	writeView(t, root, "page.html", `{{component "nope" (dict)}}`)

	m := view.NewManager(view.Config{Path: root})
	_, err := m.RenderString("page", nil)
	assert.ErrorContains(t, err, "components.nope")
}

func TestDictValidation(t *testing.T) {
	root := t.TempDir()
	writeView(t, root, "odd.html", `{{dict "only-key"}}`)
	writeView(t, root, "badkey.html", `{{dict 1 "v"}}`)

	m := view.NewManager(view.Config{Path: root})
	_, err := m.RenderString("odd", nil)
	assert.ErrorContains(t, err, "odd number")
	_, err = m.RenderString("badkey", nil)
	assert.ErrorContains(t, err, "keys must be strings")
}

// A component is a fragment of the page that calls it, so it must not
// pick up the configured default layout on the way out. It used to:
// every tag, badge and alert arrived wrapped in a second copy of the
// site's chrome, and only a page that rendered a component below the
// fold made it obvious.
func TestComponentsDoNotCarryTheDefaultLayout(t *testing.T) {
	root := t.TempDir()
	writeView(t, root, "layouts/app.html",
		`<html><body><nav>chrome</nav>{{.content}}</body></html>`)
	writeComponent(t, root, "tag.html", `<span class="tag">{{.name}}</span>`)
	writeView(t, root, "page.html", `<h1>Page</h1>{{component "tag" (dict "name" "Go")}}`)

	m := view.NewManager(view.Config{Path: root, Layout: "layouts.app"})
	out, err := m.RenderString("page", nil)
	require.NoError(t, err)

	assert.Equal(t,
		`<html><body><nav>chrome</nav><h1>Page</h1><span class="tag">Go</span></body></html>`,
		out)
	assert.Equal(t, 1, strings.Count(out, "<nav>chrome</nav>"),
		"the layout is rendered once, around the page - not again around each component")
}
