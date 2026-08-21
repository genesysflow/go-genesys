package lang_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/genesysflow/go-genesys/lang"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeLang(t *testing.T, root, locale, file, content string) {
	t.Helper()
	dir := filepath.Join(root, locale)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, file), []byte(content), 0o644))
}

func TestTransWithPlaceholders(t *testing.T) {
	root := t.TempDir()
	writeLang(t, root, "en", "messages.yaml", "welcome: \"Welcome, :name!\"\nnested:\n  deep: \"found\"\n")
	writeLang(t, root, "es", "messages.yaml", "welcome: \"¡Bienvenido, :name!\"\n")

	translator := lang.New(lang.Config{Path: root, Locale: "es", Fallback: "en"})

	assert.Equal(t, "¡Bienvenido, Ana!", translator.Trans("messages.welcome", map[string]string{"name": "Ana"}))

	// Fallback locale serves keys missing from the active locale.
	assert.Equal(t, "found", translator.Trans("messages.nested.deep"))

	// Missing keys return the key itself.
	assert.Equal(t, "messages.missing", translator.Trans("messages.missing"))
	assert.False(t, translator.Has("messages.missing"))
	assert.True(t, translator.Has("messages.welcome"))
}

func TestSetLocale(t *testing.T) {
	root := t.TempDir()
	writeLang(t, root, "en", "app.yaml", "greeting: hello\n")
	writeLang(t, root, "de", "app.yaml", "greeting: hallo\n")

	translator := lang.New(lang.Config{Path: root})
	assert.Equal(t, "hello", translator.Trans("app.greeting"))
	translator.SetLocale("de")
	assert.Equal(t, "hallo", translator.Trans("app.greeting"))
}

func TestTransChoice(t *testing.T) {
	translator := lang.New(lang.Config{Path: t.TempDir()})
	translator.AddLine("en", "apples", "one apple|:count apples")

	assert.Equal(t, "one apple", translator.TransChoice("apples", 1))
	assert.Equal(t, "5 apples", translator.TransChoice("apples", 5))
	assert.Equal(t, "0 apples", translator.TransChoice("apples", 0))
}
