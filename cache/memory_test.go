package cache

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewMemoryStore(t *testing.T) {
	store := NewMemoryStore()
	assert.NotNil(t, store)
	assert.NotNil(t, store.items)
}

func TestMemoryStorePutAndGet(t *testing.T) {
	store := NewMemoryStore()

	// Test storing and retrieving a string
	err := store.Put("key1", "value1", time.Minute)
	require.NoError(t, err)

	val, err := store.Get("key1")
	require.NoError(t, err)
	assert.Equal(t, "value1", val)
}

func TestMemoryStoreGetNonExistent(t *testing.T) {
	store := NewMemoryStore()

	val, err := store.Get("nonexistent")
	require.NoError(t, err)
	assert.Nil(t, val)
}

func TestMemoryStoreExpiration(t *testing.T) {
	store := NewMemoryStore()

	// Store with very short TTL
	err := store.Put("expiring", "value", 10*time.Millisecond)
	require.NoError(t, err)

	// Should exist immediately
	val, err := store.Get("expiring")
	require.NoError(t, err)
	assert.Equal(t, "value", val)

	// Wait for expiration
	time.Sleep(20 * time.Millisecond)

	// Should be nil after expiration
	val, err = store.Get("expiring")
	require.NoError(t, err)
	assert.Nil(t, val)
}

func TestMemoryStoreForget(t *testing.T) {
	store := NewMemoryStore()

	err := store.Put("to-forget", "value", time.Minute)
	require.NoError(t, err)

	val, err := store.Get("to-forget")
	require.NoError(t, err)
	assert.Equal(t, "value", val)

	err = store.Forget("to-forget")
	require.NoError(t, err)

	val, err = store.Get("to-forget")
	require.NoError(t, err)
	assert.Nil(t, val)
}

func TestMemoryStoreFlush(t *testing.T) {
	store := NewMemoryStore()

	// Add multiple items
	store.Put("key1", "value1", time.Minute)
	store.Put("key2", "value2", time.Minute)
	store.Put("key3", "value3", time.Minute)

	err := store.Flush()
	require.NoError(t, err)

	// All should be nil
	val1, _ := store.Get("key1")
	val2, _ := store.Get("key2")
	val3, _ := store.Get("key3")

	assert.Nil(t, val1)
	assert.Nil(t, val2)
	assert.Nil(t, val3)
}

func TestMemoryStoreDifferentTypes(t *testing.T) {
	store := NewMemoryStore()

	testCases := []struct {
		name  string
		key   string
		value any
	}{
		{"string", "str", "hello"},
		{"int", "num", 42},
		{"float", "float", 3.14},
		{"bool", "bool", true},
		{"slice", "slice", []string{"a", "b", "c"}},
		{"map", "map", map[string]int{"a": 1, "b": 2}},
		{"struct", "struct", struct{ Name string }{"test"}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := store.Put(tc.key, tc.value, time.Minute)
			require.NoError(t, err)

			val, err := store.Get(tc.key)
			require.NoError(t, err)
			assert.Equal(t, tc.value, val)
		})
	}
}

func TestMemoryStoreOverwrite(t *testing.T) {
	store := NewMemoryStore()

	err := store.Put("key", "original", time.Minute)
	require.NoError(t, err)

	err = store.Put("key", "updated", time.Minute)
	require.NoError(t, err)

	val, err := store.Get("key")
	require.NoError(t, err)
	assert.Equal(t, "updated", val)
}

func TestMemoryStoreConcurrency(t *testing.T) {
	store := NewMemoryStore()
	done := make(chan bool)

	// Run concurrent reads and writes
	for i := 0; i < 100; i++ {
		go func(n int) {
			key := "key"
			store.Put(key, n, time.Minute)
			store.Get(key)
			done <- true
		}(i)
	}

	for i := 0; i < 100; i++ {
		<-done
	}

	// Should not panic, and key should exist
	val, err := store.Get("key")
	require.NoError(t, err)
	assert.NotNil(t, val)
}

func TestMemoryStoreForgetNonExistent(t *testing.T) {
	store := NewMemoryStore()

	// Should not error when forgetting non-existent key
	err := store.Forget("nonexistent")
	require.NoError(t, err)
}
