// Package env provides environment variable loading and access.
// It supports .env files and provides type-safe helper functions.
package env

import (
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/joho/godotenv"
)

var (
	loaded bool
	mu     sync.RWMutex
)

// Load loads environment variables from .env files.
// If no paths are provided, it loads from ".env" in the current directory.
func Load(paths ...string) error {
	mu.Lock()
	defer mu.Unlock()

	if len(paths) == 0 {
		paths = []string{".env"}
	}

	err := godotenv.Load(paths...)
	if err != nil {
		// It's okay if .env file doesn't exist
		if os.IsNotExist(err) {
			loaded = true
			return nil
		}
		return err
	}

	loaded = true
	return nil
}

// LoadIfExists loads environment variables only if the files exist.
func LoadIfExists(paths ...string) error {
	mu.Lock()
	defer mu.Unlock()

	if len(paths) == 0 {
		paths = []string{".env"}
	}

	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			if err := godotenv.Load(path); err != nil {
				return err
			}
		}
	}

	loaded = true
	return nil
}

// Overload loads and overwrites existing environment variables.
func Overload(paths ...string) error {
	mu.Lock()
	defer mu.Unlock()

	if len(paths) == 0 {
		paths = []string{".env"}
	}

	err := godotenv.Overload(paths...)
	if err != nil {
		if os.IsNotExist(err) {
			loaded = true
			return nil
		}
		return err
	}

	loaded = true
	return nil
}

// IsLoaded returns true if environment has been loaded.
func IsLoaded() bool {
	mu.RLock()
	defer mu.RUnlock()
	return loaded
}

// Get retrieves an environment variable value.
// Returns the default value if the variable is not set.
func Get(key string, defaultValue ...string) string {
	value := os.Getenv(key)
	if value == "" && len(defaultValue) > 0 {
		return defaultValue[0]
	}
	return value
}

// GetString is an alias for Get.
func GetString(key string, defaultValue ...string) string {
	return Get(key, defaultValue...)
}

// GetInt retrieves an environment variable as an integer.
func GetInt(key string, defaultValue ...int) int {
	value := os.Getenv(key)
	if value == "" {
		if len(defaultValue) > 0 {
			return defaultValue[0]
		}
		return 0
	}

	intValue, err := strconv.Atoi(value)
	if err != nil {
		if len(defaultValue) > 0 {
			return defaultValue[0]
		}
		return 0
	}
	return intValue
}

// GetInt64 retrieves an environment variable as an int64.
func GetInt64(key string, defaultValue ...int64) int64 {
	value := os.Getenv(key)
	if value == "" {
		if len(defaultValue) > 0 {
			return defaultValue[0]
		}
		return 0
	}

	intValue, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		if len(defaultValue) > 0 {
			return defaultValue[0]
		}
		return 0
	}
	return intValue
}

// GetFloat retrieves an environment variable as a float64.
func GetFloat(key string, defaultValue ...float64) float64 {
	value := os.Getenv(key)
	if value == "" {
		if len(defaultValue) > 0 {
			return defaultValue[0]
		}
		return 0
	}

	floatValue, err := strconv.ParseFloat(value, 64)
	if err != nil {
		if len(defaultValue) > 0 {
			return defaultValue[0]
		}
		return 0
	}
	return floatValue
}

// GetBool retrieves an environment variable as a boolean.
// Truthy values: "true", "1", "yes", "on"
// Falsy values: "false", "0", "no", "off", ""
func GetBool(key string, defaultValue ...bool) bool {
	value := strings.ToLower(os.Getenv(key))
	if value == "" {
		if len(defaultValue) > 0 {
			return defaultValue[0]
		}
		return false
	}

	switch value {
	case "true", "1", "yes", "on":
		return true
	case "false", "0", "no", "off":
		return false
	default:
		if len(defaultValue) > 0 {
			return defaultValue[0]
		}
		return false
	}
}

// GetSlice retrieves an environment variable as a string slice.
// Values are split by the separator (default: ",").
func GetSlice(key string, separator ...string) []string {
	value := os.Getenv(key)
	if value == "" {
		return []string{}
	}

	sep := ","
	if len(separator) > 0 {
		sep = separator[0]
	}

	parts := strings.Split(value, sep)
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

// Set sets an environment variable.
func Set(key, value string) error {
	return os.Setenv(key, value)
}

// Unset removes an environment variable.
func Unset(key string) error {
	return os.Unsetenv(key)
}

// Has checks if an environment variable is set.
func Has(key string) bool {
	_, exists := os.LookupEnv(key)
	return exists
}

// All returns all environment variables as a map.
func All() map[string]string {
	env := os.Environ()
	result := make(map[string]string, len(env))
	for _, e := range env {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) == 2 {
			result[parts[0]] = parts[1]
		}
	}
	return result
}

// Require retrieves an environment variable, panicking if not set.
func Require(key string) string {
	value, exists := os.LookupEnv(key)
	if !exists {
		panic("env: required environment variable '" + key + "' is not set")
	}
	return value
}

// RequireInt retrieves an environment variable as int, panicking if not set or invalid.
func RequireInt(key string) int {
	value := Require(key)
	intValue, err := strconv.Atoi(value)
	if err != nil {
		panic("env: environment variable '" + key + "' is not a valid integer")
	}
	return intValue
}

// RequireBool retrieves an environment variable as bool, panicking if not set.
func RequireBool(key string) bool {
	value := strings.ToLower(Require(key))
	switch value {
	case "true", "1", "yes", "on":
		return true
	case "false", "0", "no", "off":
		return false
	default:
		panic("env: environment variable '" + key + "' is not a valid boolean")
	}
}
