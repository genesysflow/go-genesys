package providers

import (
	"testing"

	"github.com/genesysflow/go-genesys/session"
	"github.com/genesysflow/go-genesys/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSessionServiceProviderRegisterDefault(t *testing.T) {
	app := testutil.NewMockApplication()
	provider := &SessionServiceProvider{}

	err := provider.Register(app)
	require.NoError(t, err)

	// Check that session manager was registered
	sessionManager := app.GetInstance("session")
	assert.NotNil(t, sessionManager)
	assert.IsType(t, &session.Manager{}, sessionManager)

	// Also check the alias
	sessionManager2 := app.GetInstance("session.manager")
	assert.NotNil(t, sessionManager2)
}

func TestSessionServiceProviderRegisterWithConfig(t *testing.T) {
	app := testutil.NewMockApplication()
	customConfig := &session.Config{
		CookieName:     "custom_session",
		CookiePath:     "/app",
		CookieDomain:   "example.com",
		CookieSecure:   true,
		CookieHTTPOnly: true,
		CookieSameSite: "strict",
	}
	provider := &SessionServiceProvider{
		Config: customConfig,
	}

	err := provider.Register(app)
	require.NoError(t, err)

	sessionManager := app.GetInstance("session")
	assert.NotNil(t, sessionManager)
}

func TestSessionServiceProviderRegisterFromAppConfig(t *testing.T) {
	cfg := testutil.NewMockConfig(map[string]any{
		"session.cookie":    "app_session",
		"session.path":      "/",
		"session.domain":    "test.com",
		"session.secure":    true,
		"session.http_only": true,
		"session.same_site": "lax",
	})
	app := testutil.NewMockApplicationWithConfig(cfg)
	provider := &SessionServiceProvider{}

	err := provider.Register(app)
	require.NoError(t, err)

	sessionManager := app.GetInstance("session")
	assert.NotNil(t, sessionManager)
}

func TestSessionServiceProviderBoot(t *testing.T) {
	app := testutil.NewMockApplication()
	provider := &SessionServiceProvider{}

	err := provider.Register(app)
	require.NoError(t, err)

	err = provider.Boot(app)
	require.NoError(t, err)
}

func TestSessionServiceProviderProvides(t *testing.T) {
	provider := &SessionServiceProvider{}
	provides := provider.Provides()

	assert.Contains(t, provides, "session")
	assert.Contains(t, provides, "session.manager")
}
