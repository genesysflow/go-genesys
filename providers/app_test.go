package providers

import (
	"testing"

	"github.com/genesysflow/go-genesys/env"
	"github.com/genesysflow/go-genesys/errors"
	"github.com/genesysflow/go-genesys/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAppServiceProviderRegister(t *testing.T) {
	app := testutil.NewMockApplication()
	provider := &AppServiceProvider{}

	err := provider.Register(app)
	require.NoError(t, err)

	// Check that env helper was registered
	envHelper := app.GetInstance("env")
	assert.NotNil(t, envHelper)
	assert.IsType(t, &env.EnvHelper{}, envHelper)

	// Check that error handler was registered
	errorHandler := app.GetInstance("error.handler")
	assert.NotNil(t, errorHandler)
	assert.IsType(t, &errors.Handler{}, errorHandler)
}

func TestAppServiceProviderBoot(t *testing.T) {
	app := testutil.NewMockApplication()
	provider := &AppServiceProvider{}

	err := provider.Register(app)
	require.NoError(t, err)

	err = provider.Boot(app)
	require.NoError(t, err)
}

func TestAppServiceProviderProvides(t *testing.T) {
	provider := &AppServiceProvider{}
	provides := provider.Provides()

	assert.Contains(t, provides, "config")
	assert.Contains(t, provides, "env")
	assert.Contains(t, provides, "error.handler")
}
