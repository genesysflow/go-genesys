package view_test

import (
	"strings"
	"testing"

	"github.com/genesysflow/go-genesys/view"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCustomFuncsAndBuiltins(t *testing.T) {
	root := t.TempDir()
	writeView(t, root, "funcs.html", `{{upper .a}}|{{lower .b}}|{{shout .c}}`)

	m := view.NewManager(view.Config{Path: root})
	m.AddFunc("shout", func(s string) string { return strings.ToUpper(s) + "!" })

	out, err := m.RenderString("funcs", map[string]any{"a": "up", "b": "DOWN", "c": "hey"})
	require.NoError(t, err)
	assert.Equal(t, "UP|down|HEY!", out)
}

func TestSharedDataOverriddenPerRender(t *testing.T) {
	root := t.TempDir()
	writeView(t, root, "shared.html", `{{.name}}`)

	m := view.NewManager(view.Config{Path: root})
	m.Share("name", "global")

	out, err := m.RenderString("shared", nil)
	require.NoError(t, err)
	assert.Equal(t, "global", out)

	out, err = m.RenderString("shared", map[string]any{"name": "local"})
	require.NoError(t, err)
	assert.Equal(t, "local", out, "render data wins over shared data")
}

func TestBrokenTemplateSurfacesParseError(t *testing.T) {
	root := t.TempDir()
	writeView(t, root, "broken.html", `{{if .x}}unclosed`)

	m := view.NewManager(view.Config{Path: root})
	_, err := m.RenderString("broken", nil)
	assert.ErrorContains(t, err, "broken.html")
}

func TestRenderToWriter(t *testing.T) {
	root := t.TempDir()
	writeView(t, root, "w.html", "to writer")

	m := view.NewManager(view.Config{Path: root})
	var sb strings.Builder
	require.NoError(t, m.Render(&sb, "w", nil))
	assert.Equal(t, "to writer", sb.String())
}
