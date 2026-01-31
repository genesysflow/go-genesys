package session

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	assert.Equal(t, "genesys_session", cfg.CookieName)
	assert.Equal(t, "/", cfg.CookiePath)
	assert.False(t, cfg.CookieSecure)
	assert.True(t, cfg.CookieHTTPOnly)
	assert.Equal(t, "Lax", cfg.CookieSameSite)
	assert.Equal(t, 24*time.Hour, cfg.Expiration)
}

func TestNewManager(t *testing.T) {
	cfg := DefaultConfig()
	manager := NewManager(cfg)

	require.NotNil(t, manager)
}

func TestNewManagerWithCustomConfig(t *testing.T) {
	cfg := Config{
		CookieName:     "custom_session",
		CookiePath:     "/app",
		CookieDomain:   "example.com",
		CookieSecure:   true,
		CookieHTTPOnly: true,
		CookieSameSite: "strict",
		Expiration:     60 * time.Minute,
	}

	manager := NewManager(cfg)
	require.NotNil(t, manager)
}

func TestConfigOptions(t *testing.T) {
	tests := []struct {
		name     string
		cfg      Config
		expected Config
	}{
		{
			name: "with custom cookie name",
			cfg: Config{
				CookieName: "my_session",
			},
			expected: Config{
				CookieName: "my_session",
			},
		},
		{
			name: "with secure cookie",
			cfg: Config{
				CookieSecure: true,
			},
			expected: Config{
				CookieSecure: true,
			},
		},
		{
			name: "with custom expiration",
			cfg: Config{
				Expiration: 30 * time.Minute,
			},
			expected: Config{
				Expiration: 30 * time.Minute,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager := NewManager(tt.cfg)
			require.NotNil(t, manager)
		})
	}
}

func TestMiddlewareCreation(t *testing.T) {
	cfg := DefaultConfig()
	manager := NewManager(cfg)

	// Middleware returns a function that needs a Fiber context
	// This test just ensures it can be created
	middleware := manager.Middleware()
	assert.NotNil(t, middleware)
}
