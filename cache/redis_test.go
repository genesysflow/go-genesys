package cache_test

import (
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/genesysflow/go-genesys/cache"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newRedisStore(t *testing.T) (*cache.RedisStore, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	store := cache.NewRedisStoreWithClient(client, "")
	t.Cleanup(func() { client.Close() })
	return store, mr
}

func TestRedisStoreRoundTrip(t *testing.T) {
	store, _ := newRedisStore(t)

	require.NoError(t, store.Put("greeting", "hello", time.Minute))
	value, err := store.Get("greeting")
	require.NoError(t, err)
	assert.Equal(t, "hello", value)

	has, err := store.Has("greeting")
	require.NoError(t, err)
	assert.True(t, has)

	missing, err := store.Get("nope")
	require.NoError(t, err)
	assert.Nil(t, missing)

	// JSON round-trip: numbers come back as float64, like encoding/json.
	require.NoError(t, store.Put("n", 42, time.Minute))
	n, err := store.Get("n")
	require.NoError(t, err)
	assert.EqualValues(t, 42.0, n)

	require.NoError(t, store.Put("map", map[string]any{"a": true}, time.Minute))
	m, err := store.Get("map")
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"a": true}, m)
}

func TestRedisStoreExpiration(t *testing.T) {
	store, mr := newRedisStore(t)

	require.NoError(t, store.Put("short", "lived", time.Second))
	mr.FastForward(2 * time.Second)

	value, err := store.Get("short")
	require.NoError(t, err)
	assert.Nil(t, value, "expired keys read as missing")

	require.NoError(t, store.Forever("pinned", "stays"))
	mr.FastForward(time.Hour)
	value, err = store.Get("pinned")
	require.NoError(t, err)
	assert.Equal(t, "stays", value)
}

func TestRedisStoreAddPullForget(t *testing.T) {
	store, _ := newRedisStore(t)

	stored, err := store.Add("once", 1, time.Minute)
	require.NoError(t, err)
	assert.True(t, stored)
	stored, err = store.Add("once", 2, time.Minute)
	require.NoError(t, err)
	assert.False(t, stored, "Add refuses to overwrite")

	value, err := store.Pull("once")
	require.NoError(t, err)
	assert.EqualValues(t, 1.0, value)
	value, err = store.Get("once")
	require.NoError(t, err)
	assert.Nil(t, value, "Pull removed the key")

	require.NoError(t, store.Put("gone", "x", time.Minute))
	require.NoError(t, store.Forget("gone"))
	has, err := store.Has("gone")
	require.NoError(t, err)
	assert.False(t, has)
}

func TestRedisStoreIncrementDecrement(t *testing.T) {
	store, _ := newRedisStore(t)

	n, err := store.Increment("hits", 5)
	require.NoError(t, err)
	assert.EqualValues(t, 5, n)
	n, err = store.Decrement("hits", 2)
	require.NoError(t, err)
	assert.EqualValues(t, 3, n)

	// Incremented values read back as numbers.
	value, err := store.Get("hits")
	require.NoError(t, err)
	assert.EqualValues(t, 3.0, value)
}

func TestRedisStoreFlushOnlyTouchesPrefix(t *testing.T) {
	store, mr := newRedisStore(t)

	require.NoError(t, store.Put("mine", 1, time.Minute))
	require.NoError(t, mr.Set("unrelated", "keep"))

	require.NoError(t, store.Flush())

	has, err := store.Has("mine")
	require.NoError(t, err)
	assert.False(t, has)
	kept, _ := mr.Get("unrelated")
	assert.Equal(t, "keep", kept, "keys outside the prefix survive Flush")
}

func TestRedisStoreRememberAndManager(t *testing.T) {
	store, _ := newRedisStore(t)

	calls := 0
	value, err := cache.Remember(store, "exp", time.Minute, func() (any, error) { calls++; return "v", nil })
	require.NoError(t, err)
	assert.Equal(t, "v", value)
	_, err = cache.Remember(store, "exp", time.Minute, func() (any, error) { calls++; return "v", nil })
	require.NoError(t, err)
	assert.Equal(t, 1, calls)

	// Manager builds a redis store from config.
	mr := miniredis.RunT(t)
	manager := cache.NewManager()
	manager.Configure(cache.Config{
		Default: "redis",
		Stores:  map[string]cache.StoreConfig{"redis": {Driver: "redis", Addr: mr.Addr()}},
	})
	built, err := manager.Store()
	require.NoError(t, err)
	assert.IsType(t, &cache.RedisStore{}, built)
	require.NoError(t, built.Put("k", "v", time.Minute))
	got, err := built.Get("k")
	require.NoError(t, err)
	assert.Equal(t, "v", got)
}
