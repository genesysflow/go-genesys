package env

// EnvHelper provides convenient environment access methods.
type EnvHelper struct{}

// NewHelper creates a new EnvHelper.
func NewHelper() *EnvHelper {
	return &EnvHelper{}
}

// Get retrieves an environment variable.
func (h *EnvHelper) Get(key string, defaultValue ...string) string {
	return Get(key, defaultValue...)
}

// GetInt retrieves an environment variable as integer.
func (h *EnvHelper) GetInt(key string, defaultValue ...int) int {
	return GetInt(key, defaultValue...)
}

// GetBool retrieves an environment variable as boolean.
func (h *EnvHelper) GetBool(key string, defaultValue ...bool) bool {
	return GetBool(key, defaultValue...)
}

// GetFloat retrieves an environment variable as float.
func (h *EnvHelper) GetFloat(key string, defaultValue ...float64) float64 {
	return GetFloat(key, defaultValue...)
}

// Has checks if an environment variable is set.
func (h *EnvHelper) Has(key string) bool {
	return Has(key)
}

// Set sets an environment variable.
func (h *EnvHelper) Set(key, value string) error {
	return Set(key, value)
}

// Require retrieves an environment variable, panicking if not set.
func (h *EnvHelper) Require(key string) string {
	return Require(key)
}

