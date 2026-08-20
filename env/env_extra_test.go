package env_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/genesysflow/go-genesys/env"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTypedGetters(t *testing.T) {
	t.Setenv("TEST_STR", "hello")
	t.Setenv("TEST_INT", "42")
	t.Setenv("TEST_INT64", "9000000000")
	t.Setenv("TEST_FLOAT", "3.14")
	t.Setenv("TEST_BOOL", "true")
	t.Setenv("TEST_SLICE", "a,b,c")

	assert.Equal(t, "hello", env.Get("TEST_STR"))
	assert.Equal(t, "hello", env.GetString("TEST_STR"))
	assert.Equal(t, "fallback", env.Get("TEST_MISSING", "fallback"))
	assert.Equal(t, 42, env.GetInt("TEST_INT"))
	assert.Equal(t, 7, env.GetInt("TEST_MISSING", 7))
	assert.EqualValues(t, 9000000000, env.GetInt64("TEST_INT64"))
	assert.InDelta(t, 3.14, env.GetFloat("TEST_FLOAT"), 0.001)
	assert.True(t, env.GetBool("TEST_BOOL"))
	assert.False(t, env.GetBool("TEST_MISSING"))
	assert.True(t, env.GetBool("TEST_MISSING", true))
	assert.Equal(t, []string{"a", "b", "c"}, env.GetSlice("TEST_SLICE"))

	assert.True(t, env.Has("TEST_STR"))
	assert.False(t, env.Has("TEST_MISSING"))
}

func TestSetUnset(t *testing.T) {
	require.NoError(t, env.Set("TEST_TEMP", "value"))
	assert.Equal(t, "value", env.Get("TEST_TEMP"))
	require.NoError(t, env.Unset("TEST_TEMP"))
	assert.False(t, env.Has("TEST_TEMP"))
}

func TestRequire(t *testing.T) {
	t.Setenv("TEST_REQUIRED", "present")
	assert.Equal(t, "present", env.Require("TEST_REQUIRED"))
	assert.Panics(t, func() { env.Require("TEST_DEFINITELY_MISSING") })
}

func TestLoadFromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	require.NoError(t, os.WriteFile(path, []byte("FROM_FILE=loaded\n"), 0o600))

	require.NoError(t, env.Load(path))
	t.Cleanup(func() { os.Unsetenv("FROM_FILE") })
	assert.Equal(t, "loaded", env.Get("FROM_FILE"))

	// LoadIfExists tolerates a missing file.
	assert.NoError(t, env.LoadIfExists(filepath.Join(dir, "missing.env")))
}
