package cache

import (
	"fmt"
	"sync"
)

// Manager manages cache stores.
type Manager struct {
	stores       map[string]Store
	defaultStore string
	mu           sync.RWMutex
}

// NewManager creates a new cache manager.
func NewManager() *Manager {
	return &Manager{
		stores:       make(map[string]Store),
		defaultStore: "memory",
	}
}

// Store returns a cache store by name.
func (m *Manager) Store(name ...string) (Store, error) {
	storeName := m.defaultStore
	if len(name) > 0 && name[0] != "" {
		storeName = name[0]
	}

	m.mu.RLock()
	store, ok := m.stores[storeName]
	m.mu.RUnlock()

	if ok {
		return store, nil
	}

	// Create store if not exists (lazy loading could be implemented here)
	// For now, we only support memory store and it should be registered manually or via config
	// But to make it work out of the box:
	if storeName == "memory" {
		m.mu.Lock()
		defer m.mu.Unlock()
		// Double check
		if store, ok := m.stores[storeName]; ok {
			return store, nil
		}
		store = NewMemoryStore()
		m.stores[storeName] = store
		return store, nil
	}

	return nil, fmt.Errorf("cache store [%s] not found", storeName)
}

// Register registers a cache store.
func (m *Manager) Register(name string, store Store) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stores[name] = store
}
