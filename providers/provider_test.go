package providers

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/genesysflow/go-genesys/testutil"
)

func TestBaseProviderSetApp(t *testing.T) {
	app := testutil.NewMockApplication()
	provider := &BaseProvider{}

	provider.SetApp(app)

	assert.Equal(t, app, provider.App())
}

func TestBaseProviderApp(t *testing.T) {
	provider := &BaseProvider{}

	// Before SetApp, should return nil
	assert.Nil(t, provider.App())
}

func TestBaseProviderRegister(t *testing.T) {
	app := testutil.NewMockApplication()
	provider := &BaseProvider{}

	err := provider.Register(app)
	require.NoError(t, err)

	// After register, app should be set
	assert.Equal(t, app, provider.App())
}

func TestBaseProviderBoot(t *testing.T) {
	app := testutil.NewMockApplication()
	provider := &BaseProvider{}

	err := provider.Boot(app)
	require.NoError(t, err)
}

func TestBaseProviderProvides(t *testing.T) {
	provider := &BaseProvider{}
	provides := provider.Provides()

	// Base provider provides nothing
	assert.Empty(t, provides)
}

func TestDeferrableProviderIsDeferred(t *testing.T) {
	provider := &DeferrableProvider{}

	// Default should be false
	assert.False(t, provider.IsDeferred())
}

func TestDeferrableProviderSetDeferred(t *testing.T) {
	provider := &DeferrableProvider{}

	provider.SetDeferred(true)
	assert.True(t, provider.IsDeferred())

	provider.SetDeferred(false)
	assert.False(t, provider.IsDeferred())
}
