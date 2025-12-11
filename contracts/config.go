package contracts

// Config defines the interface for configuration management.
type Config interface {
	// Get retrieves a configuration value by key using dot notation.
	// Example: config.Get("app.name")
	Get(key string) any

	// GetString retrieves a string configuration value.
	GetString(key string) string

	// GetInt retrieves an integer configuration value.
	GetInt(key string) int

	// GetBool retrieves a boolean configuration value.
	GetBool(key string) bool

	// GetFloat retrieves a float configuration value.
	GetFloat(key string) float64

	// GetSlice retrieves a slice configuration value.
	GetSlice(key string) []any

	// GetMap retrieves a map configuration value.
	GetMap(key string) map[string]any

	// GetStringSlice retrieves a string slice configuration value.
	GetStringSlice(key string) []string

	// GetStringMap retrieves a string map configuration value.
	GetStringMap(key string) map[string]string

	// Set sets a configuration value.
	Set(key string, value any)

	// Has checks if a configuration key exists.
	Has(key string) bool

	// All returns all configuration values.
	All() map[string]any

	// Load loads configuration from a file or directory.
	Load(path string) error
}
