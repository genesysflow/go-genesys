package cache

import (
	"fmt"
	"sync"
	"time"
)

type item struct {
	value     any
	expiresAt time.Time // zero means no expiration
}

func (i item) expired() bool {
	return !i.expiresAt.IsZero() && time.Now().After(i.expiresAt)
}

// MemoryStore is an in-memory cache store.
type MemoryStore struct {
	items map[string]item
	mu    sync.Mutex
}

// NewMemoryStore creates a new memory store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		items: make(map[string]item),
	}
}

// Get retrieves an item from the cache.
func (s *MemoryStore) Get(key string) (any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	it, ok := s.items[key]
	if !ok || it.expired() {
		delete(s.items, key)
		return nil, nil
	}
	return it.value, nil
}

// Put stores an item in the cache.
func (s *MemoryStore) Put(key string, value any, ttl time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[key] = newItem(value, ttl)
	return nil
}

// Has reports whether a non-expired item exists.
func (s *MemoryStore) Has(key string) (bool, error) {
	value, err := s.Get(key)
	return value != nil, err
}

// Add stores the item only when absent; returns true when stored.
func (s *MemoryStore) Add(key string, value any, ttl time.Duration) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if it, ok := s.items[key]; ok && !it.expired() {
		return false, nil
	}
	s.items[key] = newItem(value, ttl)
	return true, nil
}

// Pull retrieves an item and removes it.
func (s *MemoryStore) Pull(key string) (any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	it, ok := s.items[key]
	delete(s.items, key)
	if !ok || it.expired() {
		return nil, nil
	}
	return it.value, nil
}

// Forever stores an item without expiration.
func (s *MemoryStore) Forever(key string, value any) error {
	return s.Put(key, value, 0)
}

// Increment increases a numeric item, starting from zero when missing.
func (s *MemoryStore) Increment(key string, amount int64) (int64, error) {
	return s.incrementBy(key, amount)
}

// Decrement decreases a numeric item, starting from zero when missing.
func (s *MemoryStore) Decrement(key string, amount int64) (int64, error) {
	return s.incrementBy(key, -amount)
}

func (s *MemoryStore) incrementBy(key string, amount int64) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var current int64
	var expiresAt time.Time
	if it, ok := s.items[key]; ok && !it.expired() {
		n, err := toNumber(it.value)
		if err != nil {
			return 0, err
		}
		current = n
		expiresAt = it.expiresAt
	}
	current += amount
	s.items[key] = item{value: current, expiresAt: expiresAt}
	return current, nil
}

// ForgetIfEquals atomically removes the key only while it still holds
// the given string value; the lock helper uses it so a Release can
// never free a lock that expired and was re-acquired by another owner.
func (s *MemoryStore) ForgetIfEquals(key string, value string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	it, ok := s.items[key]
	if !ok || it.expired() {
		return false, nil
	}
	if current, ok := it.value.(string); !ok || current != value {
		return false, nil
	}
	delete(s.items, key)
	return true, nil
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

func newItem(value any, ttl time.Duration) item {
	it := item{value: value}
	if ttl > 0 {
		it.expiresAt = time.Now().Add(ttl)
	}
	return it
}

func toNumber(value any) (int64, error) {
	switch n := value.(type) {
	case int64:
		return n, nil
	case int:
		return int64(n), nil
	case int32:
		return int64(n), nil
	case float64:
		return int64(n), nil
	case float32:
		return int64(n), nil
	}
	return 0, fmt.Errorf("cache: value is not numeric (%T)", value)
}
