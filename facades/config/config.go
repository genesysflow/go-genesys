// Package config provides a static facade for application configuration.
package config

import (
	"sync"

	"github.com/genesysflow/go-genesys/contracts"
)

var (
	instance contracts.Config
	mu       sync.RWMutex
)

// SetInstance sets the config instance.
// This is called during application bootstrap.
func SetInstance(config contracts.Config) {
	mu.Lock()
	defer mu.Unlock()
	instance = config
}

// GetInstance returns the config instance.
func GetInstance() contracts.Config {
	mu.RLock()
	defer mu.RUnlock()
	return instance
}

func config() contracts.Config {
	mu.RLock()
	defer mu.RUnlock()
	if instance == nil {
		panic("config: facade not initialised - boot the application first")
	}
	return instance
}

// Get returns a config value by dot-notation key.
func Get(key string) any { return config().Get(key) }

// GetString returns a string config value.
func GetString(key string, defaultValue ...string) string {
	value := config().GetString(key)
	if value == "" && len(defaultValue) > 0 {
		return defaultValue[0]
	}
	return value
}

// GetInt returns an integer config value.
func GetInt(key string) int { return config().GetInt(key) }

// GetBool returns a boolean config value.
func GetBool(key string) bool { return config().GetBool(key) }

// Has reports whether a key exists.
func Has(key string) bool { return config().Has(key) }
