package env_test

import (
	"os"
	"testing"

	"github.com/genesysflow/go-genesys/env"
	"github.com/stretchr/testify/assert"
)

func TestGet(t *testing.T) {
	os.Setenv("TEST_KEY", "value")
	defer os.Unsetenv("TEST_KEY")

	t.Run("it retrieves existing key", func(t *testing.T) {
		assert.Equal(t, "value", env.Get("TEST_KEY"))
	})

	t.Run("it returns default", func(t *testing.T) {
		assert.Equal(t, "default", env.Get("MISSING_KEY", "default"))
	})

	t.Run("it returns empty string if missing and no default", func(t *testing.T) {
		assert.Equal(t, "", env.Get("MISSING_KEY"))
	})
}

func TestGetInt(t *testing.T) {
	os.Setenv("TEST_INT", "123")
	os.Setenv("TEST_INVALID_INT", "abc")
	defer func() {
		os.Unsetenv("TEST_INT")
		os.Unsetenv("TEST_INVALID_INT")
	}()

	t.Run("it retrieves integer", func(t *testing.T) {
		assert.Equal(t, 123, env.GetInt("TEST_INT"))
	})

	t.Run("it returns default for missing", func(t *testing.T) {
		assert.Equal(t, 456, env.GetInt("MISSING_INT", 456))
	})

	t.Run("it returns default for invalid", func(t *testing.T) {
		assert.Equal(t, 789, env.GetInt("TEST_INVALID_INT", 789))
	})
}

func TestGetBool(t *testing.T) {
	tests := []struct {
		val      string
		expected bool
	}{
		{"true", true},
		{"1", true},
		{"on", true},
		{"yes", true},
		{"false", false},
		{"0", false},
		{"off", false},
		{"no", false},
	}

	for _, tt := range tests {
		os.Setenv("TEST_BOOL", tt.val)
		assert.Equal(t, tt.expected, env.GetBool("TEST_BOOL"), "Value: %s", tt.val)
	}
	os.Unsetenv("TEST_BOOL")

	t.Run("it returns default", func(t *testing.T) {
		assert.True(t, env.GetBool("MISSING_BOOL", true))
		assert.False(t, env.GetBool("MISSING_BOOL", false))
	})
}

func TestRequirements(t *testing.T) {
	os.Setenv("TEST_REQ", "val")
	defer os.Unsetenv("TEST_REQ")

	assert.NotPanics(t, func() {
		env.Require("TEST_REQ")
	})

	assert.Panics(t, func() {
		env.Require("MISSING_REQ")
	})
}

func TestLoad(t *testing.T) {
	content := []byte("TEST_LOADED_KEY=loaded_value")
	tmpfile, err := os.CreateTemp("", ".env")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name()) // clean up

	if _, err := tmpfile.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := tmpfile.Close(); err != nil {
		t.Fatal(err)
	}

	err = env.Load(tmpfile.Name())
	assert.NoError(t, err)

	assert.Equal(t, "loaded_value", os.Getenv("TEST_LOADED_KEY"))
	os.Unsetenv("TEST_LOADED_KEY")
}
