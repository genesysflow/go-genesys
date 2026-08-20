package cache

import (
	"fmt"
	"sync"
)

// Config configures the cache manager.
type Config struct {
	// Default is the name of the default store.
	Default string `yaml:"default" json:"default"`

	// Stores defines the available cache stores.
	Stores map[string]StoreConfig `yaml:"stores" json:"stores"`
}

// StoreConfig configures a single cache store.
type StoreConfig struct {
	// Driver is the store driver: memory or file.
	Driver string `yaml:"driver" json:"driver"`

	// Path is the storage directory for the file driver.
	Path string `yaml:"path" json:"path"`
}

// Manager manages cache stores.
type Manager struct {
	stores       map[string]Store
	configs      map[string]StoreConfig
	defaultStore string
	mu           sync.RWMutex
}

// NewManager creates a cache manager with a memory default store.
func NewManager() *Manager {
	return &Manager{
		stores:       make(map[string]Store),
		configs:      make(map[string]StoreConfig),
		defaultStore: "memory",
	}
}

// NewManagerWithConfig creates a cache manager whose stores are built
// lazily from configuration.
func NewManagerWithConfig(cfg Config) *Manager {
	m := NewManager()
	m.Configure(cfg)
	return m
}

// Configure applies store configuration to the manager. Existing stores are
// kept; configured stores are built lazily on first use.
func (m *Manager) Configure(cfg Config) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if cfg.Default != "" {
		m.defaultStore = cfg.Default
	}
	for name, storeCfg := range cfg.Stores {
		m.configs[name] = storeCfg
	}
}

// Store returns a cache store by name (default store when omitted).
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

	m.mu.Lock()
	defer m.mu.Unlock()
	if store, ok := m.stores[storeName]; ok {
		return store, nil
	}

	store, err := m.build(storeName)
	if err != nil {
		return nil, err
	}
	m.stores[storeName] = store
	return store, nil
}

// build creates a store from configuration; the memory store works with no config.
func (m *Manager) build(name string) (Store, error) {
	cfg, hasConfig := m.configs[name]
	if !hasConfig {
		if name == "memory" {
			return NewMemoryStore(), nil
		}
		return nil, fmt.Errorf("cache store [%s] not found", name)
	}

	switch cfg.Driver {
	case "memory", "":
		return NewMemoryStore(), nil
	case "file":
		path := cfg.Path
		if path == "" {
			path = "storage/cache"
		}
		return NewFileStore(path)
	default:
		return nil, fmt.Errorf("cache driver [%s] is not supported", cfg.Driver)
	}
}

// SetDefaultStore changes the default store name.
func (m *Manager) SetDefaultStore(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.defaultStore = name
}

// DefaultStore returns the default store name.
func (m *Manager) DefaultStore() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.defaultStore
}

// Register registers a pre-built cache store (e.g. a custom driver).
func (m *Manager) Register(name string, store Store) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stores[name] = store
}
