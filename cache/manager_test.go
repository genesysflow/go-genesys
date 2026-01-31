package cache

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewManager(t *testing.T) {
	manager := NewManager()
	assert.NotNil(t, manager)
	assert.NotNil(t, manager.stores)
	assert.Equal(t, "memory", manager.defaultStore)
}

func TestManagerStoreDefault(t *testing.T) {
	manager := NewManager()

	// Getting default store should create memory store
	store, err := manager.Store()
	require.NoError(t, err)
	assert.NotNil(t, store)

	// Should be able to use it
	err = store.Put("key", "value", time.Minute)
	require.NoError(t, err)

	val, err := store.Get("key")
	require.NoError(t, err)
	assert.Equal(t, "value", val)
}

func TestManagerStoreByName(t *testing.T) {
	manager := NewManager()

	// Getting store by name "memory" should work
	store, err := manager.Store("memory")
	require.NoError(t, err)
	assert.NotNil(t, store)
}

func TestManagerStoreNonExistent(t *testing.T) {
	manager := NewManager()

	// Getting non-existent store should error
	_, err := manager.Store("redis")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cache store [redis] not found")
}

func TestManagerRegister(t *testing.T) {
	manager := NewManager()

	// Register a custom store
	customStore := NewMemoryStore()
	manager.Register("custom", customStore)

	// Should be able to retrieve it
	store, err := manager.Store("custom")
	require.NoError(t, err)
	assert.Equal(t, customStore, store)
}

func TestManagerStoreCaching(t *testing.T) {
	manager := NewManager()

	// Get the default store twice
	store1, err := manager.Store()
	require.NoError(t, err)

	store2, err := manager.Store()
	require.NoError(t, err)

	// Should be the same instance
	assert.Same(t, store1, store2)
}

func TestManagerMultipleStores(t *testing.T) {
	manager := NewManager()

	// Register multiple stores
	store1 := NewMemoryStore()
	store2 := NewMemoryStore()
	manager.Register("cache1", store1)
	manager.Register("cache2", store2)

	// Put different values in each
	store1.Put("key", "value1", time.Minute)
	store2.Put("key", "value2", time.Minute)

	// Retrieve and verify
	retrieved1, _ := manager.Store("cache1")
	retrieved2, _ := manager.Store("cache2")

	val1, _ := retrieved1.Get("key")
	val2, _ := retrieved2.Get("key")

	assert.Equal(t, "value1", val1)
	assert.Equal(t, "value2", val2)
}

func TestManagerConcurrency(t *testing.T) {
	manager := NewManager()
	done := make(chan bool)

	// Concurrent store access
	for i := 0; i < 100; i++ {
		go func() {
			store, err := manager.Store()
			if err == nil {
				store.Put("key", "value", time.Minute)
				store.Get("key")
			}
			done <- true
		}()
	}

	for i := 0; i < 100; i++ {
		<-done
	}
}

func TestManagerEmptyStoreName(t *testing.T) {
	manager := NewManager()

	// Empty string should return default store
	store, err := manager.Store("")
	require.NoError(t, err)
	assert.NotNil(t, store)
}
