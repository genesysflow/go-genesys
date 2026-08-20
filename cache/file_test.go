package cache_test

import (
	"testing"
	"time"

	"github.com/genesysflow/go-genesys/cache"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stores returns one of each driver for shared behavioural tests.
func stores(t *testing.T) map[string]cache.Store {
	t.Helper()
	fileStore, err := cache.NewFileStore(t.TempDir())
	require.NoError(t, err)
	return map[string]cache.Store{
		"memory": cache.NewMemoryStore(),
		"file":   fileStore,
	}
}

func TestStoreBehaviour(t *testing.T) {
	for name, store := range stores(t) {
		t.Run(name, func(t *testing.T) {
			// Put / Get / Has
			require.NoError(t, store.Put("greeting", "hello", time.Minute))
			value, err := store.Get("greeting")
			require.NoError(t, err)
			assert.Equal(t, "hello", value)

			has, err := store.Has("greeting")
			require.NoError(t, err)
			assert.True(t, has)

			// Missing key
			value, err = store.Get("missing")
			require.NoError(t, err)
			assert.Nil(t, value)

			// Add only stores when absent
			stored, err := store.Add("greeting", "other", time.Minute)
			require.NoError(t, err)
			assert.False(t, stored)
			stored, err = store.Add("new", "value", time.Minute)
			require.NoError(t, err)
			assert.True(t, stored)

			// Pull removes
			value, err = store.Pull("new")
			require.NoError(t, err)
			assert.Equal(t, "value", value)
			has, err = store.Has("new")
			require.NoError(t, err)
			assert.False(t, has)

			// Forever + Forget
			require.NoError(t, store.Forever("pinned", "here"))
			require.NoError(t, store.Forget("pinned"))
			has, err = store.Has("pinned")
			require.NoError(t, err)
			assert.False(t, has)

			// Increment / Decrement
			n, err := store.Increment("counter", 5)
			require.NoError(t, err)
			assert.EqualValues(t, 5, n)
			n, err = store.Decrement("counter", 2)
			require.NoError(t, err)
			assert.EqualValues(t, 3, n)

			// Expiration
			require.NoError(t, store.Put("fleeting", "x", time.Millisecond))
			time.Sleep(5 * time.Millisecond)
			value, err = store.Get("fleeting")
			require.NoError(t, err)
			assert.Nil(t, value)

			// Flush
			require.NoError(t, store.Put("a", 1, time.Minute))
			require.NoError(t, store.Flush())
			value, err = store.Get("a")
			require.NoError(t, err)
			assert.Nil(t, value)
		})
	}
}

func TestRemember(t *testing.T) {
	store := cache.NewMemoryStore()

	calls := 0
	compute := func() (any, error) {
		calls++
		return "computed", nil
	}

	value, err := cache.Remember(store, "expensive", time.Minute, compute)
	require.NoError(t, err)
	assert.Equal(t, "computed", value)
	assert.Equal(t, 1, calls)

	value, err = cache.Remember(store, "expensive", time.Minute, compute)
	require.NoError(t, err)
	assert.Equal(t, "computed", value)
	assert.Equal(t, 1, calls, "second call should hit the cache")
}

func TestFileStoreExpirySurvivesRestart(t *testing.T) {
	dir := t.TempDir()
	store, err := cache.NewFileStore(dir)
	require.NoError(t, err)
	require.NoError(t, store.Put("k", "v", time.Minute))
	require.NoError(t, store.Forever("f", "always"))

	// A fresh store over the same directory sees the same data.
	reopened, err := cache.NewFileStore(dir)
	require.NoError(t, err)
	value, err := reopened.Get("k")
	require.NoError(t, err)
	assert.Equal(t, "v", value)
	value, err = reopened.Get("f")
	require.NoError(t, err)
	assert.Equal(t, "always", value)
}

func TestManagerConfiguredStores(t *testing.T) {
	manager := cache.NewManagerWithConfig(cache.Config{
		Default: "disk",
		Stores: map[string]cache.StoreConfig{
			"disk": {Driver: "file", Path: t.TempDir()},
			"mem":  {Driver: "memory"},
		},
	})

	disk, err := manager.Store()
	require.NoError(t, err)
	assert.IsType(t, &cache.FileStore{}, disk)

	mem, err := manager.Store("mem")
	require.NoError(t, err)
	assert.IsType(t, &cache.MemoryStore{}, mem)

	_, err = manager.Store("nope")
	assert.Error(t, err)
}
