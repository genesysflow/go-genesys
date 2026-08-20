package view_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/genesysflow/go-genesys/facades/view"
	baseview "github.com/genesysflow/go-genesys/view"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFacadeRender(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "hello.html"), []byte("Hello {{.who}} from {{.app}}"), 0o644))

	view.SetInstance(baseview.NewManager(baseview.Config{Path: root}))
	t.Cleanup(func() { view.SetInstance(nil) })

	view.Share("app", "Genesys")
	out, err := view.Render("hello", map[string]any{"who": "tests"})
	require.NoError(t, err)
	assert.Equal(t, "Hello tests from Genesys", out)

	assert.True(t, view.Exists("hello"))
	assert.False(t, view.Exists("nope"))
	assert.NotNil(t, view.GetInstance())
}

func TestFacadePanicsWithoutInstance(t *testing.T) {
	view.SetInstance(nil)
	assert.Panics(t, func() { view.Render("x", nil) })
}
