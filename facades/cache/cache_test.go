package cache_test

import (
	"testing"
	"time"

	basecache "github.com/genesysflow/go-genesys/cache"
	"github.com/genesysflow/go-genesys/facades/cache"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setup(t *testing.T) {
	t.Helper()
	cache.SetInstance(basecache.NewManager())
	t.Cleanup(func() { cache.SetInstance(nil) })
}

func TestFacadeRoundTrip(t *testing.T) {
	setup(t)

	require.NoError(t, cache.Put("k", "v", time.Minute))
	value, err := cache.Get("k")
	require.NoError(t, err)
	assert.Equal(t, "v", value)

	has, err := cache.Has("k")
	require.NoError(t, err)
	assert.True(t, has)

	stored, err := cache.Add("k", "other", time.Minute)
	require.NoError(t, err)
	assert.False(t, stored)

	value, err = cache.Pull("k")
	require.NoError(t, err)
	assert.Equal(t, "v", value)

	require.NoError(t, cache.Forever("pin", 1))
	n, err := cache.Increment("counter", 5)
	require.NoError(t, err)
	assert.EqualValues(t, 5, n)
	n, err = cache.Decrement("counter")
	require.NoError(t, err)
	assert.EqualValues(t, 4, n)

	calls := 0
	value, err = cache.Remember("exp", time.Minute, func() (any, error) { calls++; return 7, nil })
	require.NoError(t, err)
	assert.Equal(t, 7, value)
	_, err = cache.Remember("exp", time.Minute, func() (any, error) { calls++; return 7, nil })
	require.NoError(t, err)
	assert.Equal(t, 1, calls)

	_, err = cache.RememberForever("exp2", func() (any, error) { return 8, nil })
	require.NoError(t, err)

	require.NoError(t, cache.Forget("pin"))
	require.NoError(t, cache.Flush())

	assert.NotNil(t, cache.Store())
	assert.NotNil(t, cache.GetInstance())
}

func TestFacadePanicsWithoutInstance(t *testing.T) {
	cache.SetInstance(nil)
	assert.Panics(t, func() { cache.Get("k") })
}
