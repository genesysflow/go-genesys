package cache_test

import (
	"testing"
	"time"

	"github.com/genesysflow/go-genesys/cache"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaggedCacheIsolatesAndFlushes(t *testing.T) {
	store := cache.NewMemoryStore()

	users := cache.Tags(store, "users")
	posts := cache.Tags(store, "posts")

	require.NoError(t, users.Put("count", 42, time.Minute))
	require.NoError(t, posts.Put("count", 7, time.Minute))

	v, err := users.Get("count")
	require.NoError(t, err)
	assert.EqualValues(t, 42, v, "same key under different tags stays separate")
	v, err = posts.Get("count")
	require.NoError(t, err)
	assert.EqualValues(t, 7, v)

	// Flushing one tag leaves the other intact.
	require.NoError(t, cache.Tags(store, "users").Flush())
	v, err = users.Get("count")
	require.NoError(t, err)
	assert.Nil(t, v, "flushed tag loses its entries")
	v, err = posts.Get("count")
	require.NoError(t, err)
	assert.EqualValues(t, 7, v, "other tags are untouched")
}

func TestTaggedCacheMultiTagFlush(t *testing.T) {
	store := cache.NewMemoryStore()

	both := cache.Tags(store, "authors", "books")
	require.NoError(t, both.Put("joint", "x", time.Minute))
	onlyAuthors := cache.Tags(store, "authors")
	require.NoError(t, onlyAuthors.Put("solo", "y", time.Minute))

	// Flushing one member tag invalidates entries stored under the pair.
	require.NoError(t, cache.Tags(store, "books").Flush())
	v, err := both.Get("joint")
	require.NoError(t, err)
	assert.Nil(t, v, "an entry under {authors,books} dies with either tag")
	v, err = onlyAuthors.Get("solo")
	require.NoError(t, err)
	assert.Equal(t, "y", v, "authors-only entries survive a books flush")
}

func TestTaggedRememberAndOrder(t *testing.T) {
	store := cache.NewMemoryStore()

	calls := 0
	compute := func() (any, error) { calls++; return "value", nil }

	a, err := cache.Tags(store, "a", "b").Remember("k", time.Minute, compute)
	require.NoError(t, err)
	assert.Equal(t, "value", a)

	// Tag order must not matter for addressing.
	b, err := cache.Tags(store, "b", "a").Remember("k", time.Minute, compute)
	require.NoError(t, err)
	assert.Equal(t, "value", b)
	assert.Equal(t, 1, calls, "the second Remember hit the cache")
}
