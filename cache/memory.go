package cache

import (
	"sync"
	"time"
)

type item struct {
	value     any
	expiresAt time.Time
}

// MemoryStore is an in-memory cache store.
type MemoryStore struct {
	items map[string]item
	mu    sync.RWMutex
}

// NewMemoryStore creates a new memory store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		items: make(map[string]item),
	}
}

// Get retrieves an item from the cache.
func (s *MemoryStore) Get(key string) (any, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	item, ok := s.items[key]
	if !ok {
		return nil, nil
	}

	if time.Now().After(item.expiresAt) {
		return nil, nil
	}

	return item.value, nil
}

// Put stores an item in the cache.
func (s *MemoryStore) Put(key string, value any, ttl time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.items[key] = item{
		value:     value,
		expiresAt: time.Now().Add(ttl),
	}
	return nil
}

// Forget removes an item from the cache.
func (s *MemoryStore) Forget(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.items, key)
	return nil
}

// Flush removes all items from the cache.
func (s *MemoryStore) Flush() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items = make(map[string]item)
	return nil
}
