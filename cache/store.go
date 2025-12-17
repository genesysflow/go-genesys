package cache

import "time"

// Store is the interface for cache stores.
type Store interface {
	// Get retrieves an item from the cache.
	Get(key string) (any, error)

	// Put stores an item in the cache.
	Put(key string, value any, ttl time.Duration) error

	// Forget removes an item from the cache.
	Forget(key string) error

	// Flush removes all items from the cache.
	Flush() error
}
