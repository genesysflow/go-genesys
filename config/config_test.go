package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	cfg := New()
	assert.NotNil(t, cfg)
	assert.NotNil(t, cfg.data)
}

func TestLoadYAML(t *testing.T) {
	// Create a temp YAML file
	tmpDir := t.TempDir()
	yamlContent := `
name: test-app
debug: true
port: 8080
database:
  host: localhost
  port: 5432
  name: testdb
`
	yamlFile := filepath.Join(tmpDir, "app.yaml")
	err := os.WriteFile(yamlFile, []byte(yamlContent), 0644)
	require.NoError(t, err)

	cfg := New()
	err = cfg.Load(yamlFile)
	require.NoError(t, err)

	assert.Equal(t, "test-app", cfg.GetString("name"))
	assert.Equal(t, true, cfg.GetBool("debug"))
	assert.Equal(t, 8080, cfg.GetInt("port"))
	assert.Equal(t, "localhost", cfg.GetString("database.host"))
	assert.Equal(t, 5432, cfg.GetInt("database.port"))
	assert.Equal(t, "testdb", cfg.GetString("database.name"))
}

func TestLoadJSON(t *testing.T) {
	// Create a temp JSON file
	tmpDir := t.TempDir()
	jsonContent := `{
		"name": "json-app",
		"enabled": true,
		"count": 42,
		"nested": {
			"key": "value"
		}
	}`
	jsonFile := filepath.Join(tmpDir, "config.json")
	err := os.WriteFile(jsonFile, []byte(jsonContent), 0644)
	require.NoError(t, err)

	cfg := New()
	err = cfg.Load(jsonFile)
	require.NoError(t, err)

	assert.Equal(t, "json-app", cfg.GetString("name"))
	assert.Equal(t, true, cfg.GetBool("enabled"))
	assert.Equal(t, 42, cfg.GetInt("count"))
	assert.Equal(t, "value", cfg.GetString("nested.key"))
}

func TestLoadDirectory(t *testing.T) {
	tmpDir := t.TempDir()

	// Create multiple config files
	appYAML := `
name: my-app
env: production
`
	dbYAML := `
driver: pgsql
host: db.example.com
port: 5432
`
	err := os.WriteFile(filepath.Join(tmpDir, "app.yaml"), []byte(appYAML), 0644)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(tmpDir, "database.yaml"), []byte(dbYAML), 0644)
	require.NoError(t, err)

	cfg := New()
	err = cfg.Load(tmpDir)
	require.NoError(t, err)

	// Files are loaded with filename as key
	assert.Equal(t, "my-app", cfg.GetString("app.name"))
	assert.Equal(t, "production", cfg.GetString("app.env"))
	assert.Equal(t, "pgsql", cfg.GetString("database.driver"))
	assert.Equal(t, "db.example.com", cfg.GetString("database.host"))
	assert.Equal(t, 5432, cfg.GetInt("database.port"))
}

func TestLoadNonExistentFile(t *testing.T) {
	cfg := New()
	err := cfg.Load("/nonexistent/path")
	assert.Error(t, err)
}

func TestLoadUnsupportedFormat(t *testing.T) {
	tmpDir := t.TempDir()
	txtFile := filepath.Join(tmpDir, "config.txt")
	err := os.WriteFile(txtFile, []byte("some text"), 0644)
	require.NoError(t, err)

	cfg := New()
	err = cfg.Load(txtFile)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported file format")
}

func TestGet(t *testing.T) {
	cfg := New()
	cfg.Set("key", "value")
	cfg.Set("nested.deep.key", "deep-value")

	assert.Equal(t, "value", cfg.Get("key"))
	assert.Equal(t, "deep-value", cfg.Get("nested.deep.key"))
	assert.Nil(t, cfg.Get("nonexistent"))
}

func TestGetString(t *testing.T) {
	cfg := New()
	cfg.Set("string", "hello")
	cfg.Set("number", 42)
	cfg.Set("bool", true)

	assert.Equal(t, "hello", cfg.GetString("string"))
	assert.Equal(t, "42", cfg.GetString("number"))
	assert.Equal(t, "true", cfg.GetString("bool"))
	assert.Equal(t, "", cfg.GetString("nonexistent"))
}

func TestGetInt(t *testing.T) {
	tmpDir := t.TempDir()
	yamlContent := `
int_val: 100
float_val: 3.14
string_val: "42"
`
	yamlFile := filepath.Join(tmpDir, "test.yaml")
	err := os.WriteFile(yamlFile, []byte(yamlContent), 0644)
	require.NoError(t, err)

	cfg := New()
	err = cfg.Load(yamlFile)
	require.NoError(t, err)

	assert.Equal(t, 100, cfg.GetInt("int_val"))
	assert.Equal(t, 3, cfg.GetInt("float_val"))   // float64 to int
	assert.Equal(t, 42, cfg.GetInt("string_val")) // string to int
	assert.Equal(t, 0, cfg.GetInt("nonexistent"))
}

func TestGetInt64(t *testing.T) {
	cfg := New()
	cfg.Set("bignum", int64(9223372036854775807))
	cfg.Set("smallnum", 42)

	assert.Equal(t, int64(9223372036854775807), cfg.GetInt64("bignum"))
	assert.Equal(t, int64(42), cfg.GetInt64("smallnum"))
	assert.Equal(t, int64(0), cfg.GetInt64("nonexistent"))
}

func TestGetBool(t *testing.T) {
	tmpDir := t.TempDir()
	yamlContent := `
bool_true: true
bool_false: false
string_true: "true"
string_yes: "yes"
string_one: "1"
int_one: 1
int_zero: 0
`
	yamlFile := filepath.Join(tmpDir, "test.yaml")
	err := os.WriteFile(yamlFile, []byte(yamlContent), 0644)
	require.NoError(t, err)

	cfg := New()
	err = cfg.Load(yamlFile)
	require.NoError(t, err)

	assert.True(t, cfg.GetBool("bool_true"))
	assert.False(t, cfg.GetBool("bool_false"))
	assert.True(t, cfg.GetBool("string_true"))
	assert.True(t, cfg.GetBool("string_yes"))
	assert.True(t, cfg.GetBool("string_one"))
	assert.True(t, cfg.GetBool("int_one"))
	assert.False(t, cfg.GetBool("int_zero"))
	assert.False(t, cfg.GetBool("nonexistent"))
}

func TestGetFloat(t *testing.T) {
	cfg := New()
	cfg.Set("float", 3.14159)
	cfg.Set("int", 42)
	cfg.Set("string", "2.718")

	assert.InDelta(t, 3.14159, cfg.GetFloat("float"), 0.0001)
	assert.InDelta(t, 42.0, cfg.GetFloat("int"), 0.0001)
	assert.InDelta(t, 2.718, cfg.GetFloat("string"), 0.0001)
	assert.InDelta(t, 0.0, cfg.GetFloat("nonexistent"), 0.0001)
}

func TestGetSlice(t *testing.T) {
	tmpDir := t.TempDir()
	yamlContent := `
items:
  - one
  - two
  - three
`
	yamlFile := filepath.Join(tmpDir, "test.yaml")
	err := os.WriteFile(yamlFile, []byte(yamlContent), 0644)
	require.NoError(t, err)

	cfg := New()
	err = cfg.Load(yamlFile)
	require.NoError(t, err)

	slice := cfg.GetSlice("items")
	require.Len(t, slice, 3)
	assert.Equal(t, "one", slice[0])
	assert.Equal(t, "two", slice[1])
	assert.Equal(t, "three", slice[2])
	assert.Nil(t, cfg.GetSlice("nonexistent"))
}

func TestGetMap(t *testing.T) {
	tmpDir := t.TempDir()
	yamlContent := `
settings:
  key1: value1
  key2: value2
`
	yamlFile := filepath.Join(tmpDir, "test.yaml")
	err := os.WriteFile(yamlFile, []byte(yamlContent), 0644)
	require.NoError(t, err)

	cfg := New()
	err = cfg.Load(yamlFile)
	require.NoError(t, err)

	m := cfg.GetMap("settings")
	require.NotNil(t, m)
	assert.Equal(t, "value1", m["key1"])
	assert.Equal(t, "value2", m["key2"])
	assert.Nil(t, cfg.GetMap("nonexistent"))
}

func TestGetStringSlice(t *testing.T) {
	tmpDir := t.TempDir()
	yamlContent := `
tags:
  - alpha
  - beta
  - gamma
`
	yamlFile := filepath.Join(tmpDir, "test.yaml")
	err := os.WriteFile(yamlFile, []byte(yamlContent), 0644)
	require.NoError(t, err)

	cfg := New()
	err = cfg.Load(yamlFile)
	require.NoError(t, err)

	slice := cfg.GetStringSlice("tags")
	require.Len(t, slice, 3)
	assert.Equal(t, []string{"alpha", "beta", "gamma"}, slice)
}

func TestGetStringMap(t *testing.T) {
	tmpDir := t.TempDir()
	yamlContent := `
env:
  APP_ENV: production
  DB_HOST: localhost
`
	yamlFile := filepath.Join(tmpDir, "test.yaml")
	err := os.WriteFile(yamlFile, []byte(yamlContent), 0644)
	require.NoError(t, err)

	cfg := New()
	err = cfg.Load(yamlFile)
	require.NoError(t, err)

	m := cfg.GetStringMap("env")
	require.NotNil(t, m)
	assert.Equal(t, "production", m["APP_ENV"])
	assert.Equal(t, "localhost", m["DB_HOST"])
}

func TestSet(t *testing.T) {
	cfg := New()

	cfg.Set("simple", "value")
	cfg.Set("nested.key", "nested-value")
	cfg.Set("deep.nested.key", "deep-value")

	assert.Equal(t, "value", cfg.GetString("simple"))
	assert.Equal(t, "nested-value", cfg.GetString("nested.key"))
	assert.Equal(t, "deep-value", cfg.GetString("deep.nested.key"))
}

func TestHas(t *testing.T) {
	cfg := New()
	cfg.Set("exists", "value")
	cfg.Set("nested.exists", "value")

	assert.True(t, cfg.Has("exists"))
	assert.True(t, cfg.Has("nested.exists"))
	assert.False(t, cfg.Has("nonexistent"))
	assert.False(t, cfg.Has("nested.nonexistent"))
}

func TestAll(t *testing.T) {
	cfg := New()
	cfg.Set("key1", "value1")
	cfg.Set("key2", "value2")

	all := cfg.All()
	assert.Equal(t, "value1", all["key1"])
	assert.Equal(t, "value2", all["key2"])

	// Modifying the returned map should not affect the original
	all["key1"] = "modified"
	assert.Equal(t, "value1", cfg.GetString("key1"))
}

func TestMerge(t *testing.T) {
	cfg := New()
	cfg.Set("original", "value")
	cfg.Set("nested.key", "original-nested")

	cfg.Merge(map[string]any{
		"merged": "new-value",
		"nested": map[string]any{
			"key":  "merged-nested",
			"key2": "new-key",
		},
	})

	assert.Equal(t, "value", cfg.GetString("original"))
	assert.Equal(t, "new-value", cfg.GetString("merged"))
	// Note: Merge replaces nested maps entirely based on the implementation
	assert.Equal(t, "merged-nested", cfg.GetString("nested.key"))
	assert.Equal(t, "new-key", cfg.GetString("nested.key2"))
}

func TestEnvironmentVariableInterpolation(t *testing.T) {
	// Set environment variables
	os.Setenv("TEST_APP_NAME", "env-app")
	os.Setenv("TEST_DB_PORT", "3306")
	defer os.Unsetenv("TEST_APP_NAME")
	defer os.Unsetenv("TEST_DB_PORT")

	tmpDir := t.TempDir()
	yamlContent := `
name: ${TEST_APP_NAME}
port: ${TEST_DB_PORT}
default_value: ${NONEXISTENT_VAR:-default}
missing: ${NONEXISTENT_VAR}
`
	yamlFile := filepath.Join(tmpDir, "test.yaml")
	err := os.WriteFile(yamlFile, []byte(yamlContent), 0644)
	require.NoError(t, err)

	cfg := New()
	err = cfg.Load(yamlFile)
	require.NoError(t, err)

	assert.Equal(t, "env-app", cfg.GetString("name"))
	assert.Equal(t, "3306", cfg.GetString("port"))
	assert.Equal(t, "default", cfg.GetString("default_value"))
	assert.Equal(t, "", cfg.GetString("missing"))
}

func TestConcurrentAccess(t *testing.T) {
	cfg := New()

	// Run concurrent reads and writes
	done := make(chan bool)
	for i := 0; i < 100; i++ {
		go func(n int) {
			cfg.Set("key", n)
			_ = cfg.Get("key")
			_ = cfg.GetString("key")
			_ = cfg.Has("key")
			done <- true
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < 100; i++ {
		<-done
	}
}

func TestInvalidYAML(t *testing.T) {
	tmpDir := t.TempDir()
	invalidYAML := `
invalid: [unclosed bracket
`
	yamlFile := filepath.Join(tmpDir, "invalid.yaml")
	err := os.WriteFile(yamlFile, []byte(invalidYAML), 0644)
	require.NoError(t, err)

	cfg := New()
	err = cfg.Load(yamlFile)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse YAML")
}

func TestInvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	invalidJSON := `{"invalid": json}`
	jsonFile := filepath.Join(tmpDir, "invalid.json")
	err := os.WriteFile(jsonFile, []byte(invalidJSON), 0644)
	require.NoError(t, err)

	cfg := New()
	err = cfg.Load(jsonFile)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse JSON")
}
