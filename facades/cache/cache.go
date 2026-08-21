// Package cache provides a static facade for the cache manager.
package cache

import (
	"sync"
	"time"

	basecache "github.com/genesysflow/go-genesys/cache"
)

var (
	instance *basecache.Manager
	mu       sync.RWMutex
)

// SetInstance sets the cache manager instance.
// This is called during application bootstrap.
func SetInstance(manager *basecache.Manager) {
	mu.Lock()
	defer mu.Unlock()
	instance = manager
}

// GetInstance returns the cache manager instance.
func GetInstance() *basecache.Manager {
	mu.RLock()
	defer mu.RUnlock()
	return instance
}

func store(name ...string) basecache.Store {
	mu.RLock()
	manager := instance
	mu.RUnlock()
	if manager == nil {
		panic("cache: facade not initialised - register the CacheServiceProvider")
	}
	s, err := manager.Store(name...)
	if err != nil {
		panic(err)
	}
	return s
}

// Store returns a cache store by name (default store when omitted).
func Store(name ...string) basecache.Store {
	return store(name...)
}

// Get retrieves an item from the default store.
func Get(key string) (any, error) {
	return store().Get(key)
}

// Put stores an item in the default store.
func Put(key string, value any, ttl time.Duration) error {
	return store().Put(key, value, ttl)
}

// Has reports whether the default store holds a non-expired item.
func Has(key string) (bool, error) {
	return store().Has(key)
}

// Add stores an item only when absent; returns true when stored.
func Add(key string, value any, ttl time.Duration) (bool, error) {
	return store().Add(key, value, ttl)
}

// Pull retrieves an item and removes it.
func Pull(key string) (any, error) {
	return store().Pull(key)
}

// Forever stores an item without expiration.
func Forever(key string, value any) error {
	return store().Forever(key, value)
}

// Increment increases a numeric item and returns the new value.
func Increment(key string, amount ...int64) (int64, error) {
	n := int64(1)
	if len(amount) > 0 {
		n = amount[0]
	}
	return store().Increment(key, n)
}

// Decrement decreases a numeric item and returns the new value.
func Decrement(key string, amount ...int64) (int64, error) {
	n := int64(1)
	if len(amount) > 0 {
		n = amount[0]
	}
	return store().Decrement(key, n)
}

// Forget removes an item from the default store.
func Forget(key string) error {
	return store().Forget(key)
}

// Flush removes all items from the default store.
func Flush() error {
	return store().Flush()
}

// Remember returns the cached value, computing and storing it when absent.
func Remember(key string, ttl time.Duration, fn func() (any, error)) (any, error) {
	return basecache.Remember(store(), key, ttl, fn)
}

// RememberForever is Remember without expiration.
func RememberForever(key string, fn func() (any, error)) (any, error) {
	return basecache.RememberForever(store(), key, fn)
}
