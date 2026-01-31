package providers

import (
	"testing"

	"github.com/genesysflow/go-genesys/cache"
	"github.com/genesysflow/go-genesys/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCacheServiceProviderRegister(t *testing.T) {
	app := testutil.NewMockApplication()
	provider := &CacheServiceProvider{}

	err := provider.Register(app)
	require.NoError(t, err)

	// Check that cache manager was registered
	cacheManager := app.GetInstance("cache")
	assert.NotNil(t, cacheManager)
	assert.IsType(t, &cache.Manager{}, cacheManager)
}

func TestCacheServiceProviderBoot(t *testing.T) {
	app := testutil.NewMockApplication()
	provider := &CacheServiceProvider{}

	err := provider.Register(app)
	require.NoError(t, err)

	err = provider.Boot(app)
	require.NoError(t, err)
}

func TestCacheServiceProviderProvides(t *testing.T) {
	provider := &CacheServiceProvider{}
	provides := provider.Provides()

	assert.Contains(t, provides, "cache")
}
