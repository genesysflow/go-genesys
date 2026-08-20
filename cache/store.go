package cache

import "time"

// Store is the interface for cache stores.
type Store interface {
	// Get retrieves an item from the cache (nil when missing or expired).
	Get(key string) (any, error)

	// Put stores an item in the cache for the given TTL.
	Put(key string, value any, ttl time.Duration) error

	// Has reports whether a non-expired item exists.
	Has(key string) (bool, error)

	// Add stores an item only when the key does not already exist.
	// Returns true when the item was stored.
	Add(key string, value any, ttl time.Duration) (bool, error)

	// Pull retrieves an item and removes it from the cache.
	Pull(key string) (any, error)

	// Forever stores an item without expiration.
	Forever(key string, value any) error

	// Increment increases a numeric item by the given amount and returns
	// the new value. Missing items start at zero.
	Increment(key string, amount int64) (int64, error)

	// Decrement decreases a numeric item by the given amount and returns
	// the new value. Missing items start at zero.
	Decrement(key string, amount int64) (int64, error)

	// Forget removes an item from the cache.
	Forget(key string) error

	// Flush removes all items from the cache.
	Flush() error
}

// Remember returns the cached value for key, computing and storing it with
// the given TTL when absent:
//
//	users, err := cache.Remember(store, "users:all", time.Minute, func() (any, error) {
//	    return loadUsers()
//	})
func Remember(store Store, key string, ttl time.Duration, fn func() (any, error)) (any, error) {
	value, err := store.Get(key)
	if err != nil {
		return nil, err
	}
	if value != nil {
		return value, nil
	}
	value, err = fn()
	if err != nil {
		return nil, err
	}
	if err := store.Put(key, value, ttl); err != nil {
		return nil, err
	}
	return value, nil
}

// RememberForever is Remember without expiration.
func RememberForever(store Store, key string, fn func() (any, error)) (any, error) {
	value, err := store.Get(key)
	if err != nil {
		return nil, err
	}
	if value != nil {
		return value, nil
	}
	value, err = fn()
	if err != nil {
		return nil, err
	}
	if err := store.Forever(key, value); err != nil {
		return nil, err
	}
	return value, nil
}
