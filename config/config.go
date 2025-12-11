// Package config provides configuration management with YAML/JSON support.
// It offers dot-notation access and environment variable interpolation.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/genesysflow/go-genesys/env"
	"gopkg.in/yaml.v3"
)

// Config holds the application configuration.
type Config struct {
	data map[string]any
	mu   sync.RWMutex
}

// New creates a new Config instance.
func New() *Config {
	return &Config{
		data: make(map[string]any),
	}
}

// Load loads configuration from a file or directory.
// If path is a directory, it loads all .yaml, .yml, and .json files.
func (c *Config) Load(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("config: failed to stat path '%s': %w", path, err)
	}

	if info.IsDir() {
		return c.loadDir(path)
	}
	return c.loadFile(path, "")
}

// loadDir loads all config files from a directory.
func (c *Config) loadDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("config: failed to read directory '%s': %w", dir, err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		ext := strings.ToLower(filepath.Ext(name))
		if ext != ".yaml" && ext != ".yml" && ext != ".json" {
			continue
		}

		// Use filename without extension as the config key
		key := strings.TrimSuffix(name, ext)
		if err := c.loadFile(filepath.Join(dir, name), key); err != nil {
			return err
		}
	}

	return nil
}

// loadFile loads a single config file.
func (c *Config) loadFile(path, key string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("config: failed to read file '%s': %w", path, err)
	}

	// Interpolate environment variables
	data = []byte(interpolateEnv(string(data)))

	var parsed map[string]any
	ext := strings.ToLower(filepath.Ext(path))

	switch ext {
	case ".yaml", ".yml":
		if err := yaml.Unmarshal(data, &parsed); err != nil {
			return fmt.Errorf("config: failed to parse YAML '%s': %w", path, err)
		}
	case ".json":
		if err := json.Unmarshal(data, &parsed); err != nil {
			return fmt.Errorf("config: failed to parse JSON '%s': %w", path, err)
		}
	default:
		return fmt.Errorf("config: unsupported file format '%s'", ext)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if key == "" {
		// Merge into root
		for k, v := range parsed {
			c.data[k] = v
		}
	} else {
		c.data[key] = parsed
	}

	return nil
}

// interpolateEnv replaces ${VAR} and ${VAR:-default} with environment variable values.
var envVarRegex = regexp.MustCompile(`\$\{([^}:]+)(?::-([^}]*))?\}`)

func interpolateEnv(s string) string {
	return envVarRegex.ReplaceAllStringFunc(s, func(match string) string {
		matches := envVarRegex.FindStringSubmatch(match)
		if len(matches) < 2 {
			return match
		}

		varName := matches[1]
		defaultVal := ""
		if len(matches) >= 3 {
			defaultVal = matches[2]
		}

		return env.Get(varName, defaultVal)
	})
}

// Get retrieves a configuration value by key using dot notation.
// Example: config.Get("app.name")
func (c *Config) Get(key string) any {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return getNestedValue(c.data, key)
}

// GetString retrieves a string configuration value.
func (c *Config) GetString(key string) string {
	value := c.Get(key)
	if value == nil {
		return ""
	}
	if s, ok := value.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", value)
}

// GetInt retrieves an integer configuration value.
func (c *Config) GetInt(key string) int {
	value := c.Get(key)
	if value == nil {
		return 0
	}

	switch v := value.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case string:
		i, _ := strconv.Atoi(v)
		return i
	default:
		return 0
	}
}

// GetInt64 retrieves an int64 configuration value.
func (c *Config) GetInt64(key string) int64 {
	value := c.Get(key)
	if value == nil {
		return 0
	}

	switch v := value.(type) {
	case int:
		return int64(v)
	case int64:
		return v
	case float64:
		return int64(v)
	case string:
		i, _ := strconv.ParseInt(v, 10, 64)
		return i
	default:
		return 0
	}
}

// GetBool retrieves a boolean configuration value.
func (c *Config) GetBool(key string) bool {
	value := c.Get(key)
	if value == nil {
		return false
	}

	switch v := value.(type) {
	case bool:
		return v
	case string:
		return strings.ToLower(v) == "true" || v == "1" || v == "yes"
	case int:
		return v != 0
	case float64:
		return v != 0
	default:
		return false
	}
}

// GetFloat retrieves a float configuration value.
func (c *Config) GetFloat(key string) float64 {
	value := c.Get(key)
	if value == nil {
		return 0
	}

	switch v := value.(type) {
	case float64:
		return v
	case float32:
		return float64(v)
	case int:
		return float64(v)
	case int64:
		return float64(v)
	case string:
		f, _ := strconv.ParseFloat(v, 64)
		return f
	default:
		return 0
	}
}

// GetSlice retrieves a slice configuration value.
func (c *Config) GetSlice(key string) []any {
	value := c.Get(key)
	if value == nil {
		return nil
	}

	if slice, ok := value.([]any); ok {
		return slice
	}
	return nil
}

// GetMap retrieves a map configuration value.
func (c *Config) GetMap(key string) map[string]any {
	value := c.Get(key)
	if value == nil {
		return nil
	}

	if m, ok := value.(map[string]any); ok {
		return m
	}
	return nil
}

// GetStringSlice retrieves a string slice configuration value.
func (c *Config) GetStringSlice(key string) []string {
	value := c.GetSlice(key)
	if value == nil {
		return nil
	}

	result := make([]string, 0, len(value))
	for _, v := range value {
		if s, ok := v.(string); ok {
			result = append(result, s)
		} else {
			result = append(result, fmt.Sprintf("%v", v))
		}
	}
	return result
}

// GetStringMap retrieves a string map configuration value.
func (c *Config) GetStringMap(key string) map[string]string {
	value := c.GetMap(key)
	if value == nil {
		return nil
	}

	result := make(map[string]string, len(value))
	for k, v := range value {
		if s, ok := v.(string); ok {
			result[k] = s
		} else {
			result[k] = fmt.Sprintf("%v", v)
		}
	}
	return result
}

// Set sets a configuration value using dot notation.
func (c *Config) Set(key string, value any) {
	c.mu.Lock()
	defer c.mu.Unlock()

	setNestedValue(c.data, key, value)
}

// Has checks if a configuration key exists.
func (c *Config) Has(key string) bool {
	return c.Get(key) != nil
}

// All returns all configuration values.
func (c *Config) All() map[string]any {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Return a copy to prevent external modification
	return copyMap(c.data)
}

// Merge merges another config map into this config.
func (c *Config) Merge(data map[string]any) {
	c.mu.Lock()
	defer c.mu.Unlock()

	mergeMaps(c.data, data)
}

// getNestedValue retrieves a value from a nested map using dot notation.
func getNestedValue(data map[string]any, key string) any {
	parts := strings.Split(key, ".")
	current := any(data)

	for _, part := range parts {
		switch v := current.(type) {
		case map[string]any:
			var ok bool
			current, ok = v[part]
			if !ok {
				return nil
			}
		default:
			return nil
		}
	}

	return current
}

// setNestedValue sets a value in a nested map using dot notation.
func setNestedValue(data map[string]any, key string, value any) {
	parts := strings.Split(key, ".")
	current := data

	for i, part := range parts {
		if i == len(parts)-1 {
			current[part] = value
			return
		}

		if _, ok := current[part]; !ok {
			current[part] = make(map[string]any)
		}

		if next, ok := current[part].(map[string]any); ok {
			current = next
		} else {
			// Overwrite non-map value with new map
			newMap := make(map[string]any)
			current[part] = newMap
			current = newMap
		}
	}
}

// copyMap creates a deep copy of a map.
func copyMap(src map[string]any) map[string]any {
	dst := make(map[string]any, len(src))
	for k, v := range src {
		if m, ok := v.(map[string]any); ok {
			dst[k] = copyMap(m)
		} else if s, ok := v.([]any); ok {
			dst[k] = copySlice(s)
		} else {
			dst[k] = v
		}
	}
	return dst
}

// copySlice creates a deep copy of a slice.
func copySlice(src []any) []any {
	dst := make([]any, len(src))
	for i, v := range src {
		if m, ok := v.(map[string]any); ok {
			dst[i] = copyMap(m)
		} else if s, ok := v.([]any); ok {
			dst[i] = copySlice(s)
		} else {
			dst[i] = v
		}
	}
	return dst
}

// mergeMaps recursively merges src into dst.
func mergeMaps(dst, src map[string]any) {
	for k, v := range src {
		if dstVal, ok := dst[k]; ok {
			if dstMap, ok := dstVal.(map[string]any); ok {
				if srcMap, ok := v.(map[string]any); ok {
					mergeMaps(dstMap, srcMap)
					continue
				}
			}
		}
		dst[k] = v
	}
}

