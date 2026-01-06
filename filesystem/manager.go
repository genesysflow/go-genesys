package filesystem

import (
	"fmt"
	"sync"

	"github.com/genesysflow/go-genesys/contracts"
)

// Manager manages filesystem disks.
type Manager struct {
	config  contracts.Config
	disks   map[string]contracts.Filesystem
	drivers map[string]func(config map[string]any) (contracts.Filesystem, error)
	mu      sync.RWMutex
}

// NewManager creates a new filesystem manager.
func NewManager(config contracts.Config) *Manager {
	return &Manager{
		config:  config,
		disks:   make(map[string]contracts.Filesystem),
		drivers: make(map[string]func(config map[string]any) (contracts.Filesystem, error)),
	}
}

// Disk gets a filesystem instance by name.
func (m *Manager) Disk(name ...string) contracts.Filesystem {
	diskName := m.getDefaultDriver()
	if len(name) > 0 {
		diskName = name[0]
	}

	// First check with read lock (fast path)
	m.mu.RLock()
	if disk, ok := m.disks[diskName]; ok {
		m.mu.RUnlock()
		return disk
	}
	m.mu.RUnlock()

	// Disk doesn't exist, acquire write lock for initialization
	m.mu.Lock()
	defer m.mu.Unlock()

	// Double-check: another goroutine might have initialized it
	if disk, ok := m.disks[diskName]; ok {
		return disk
	}

	disk, err := m.resolve(diskName)
	if err != nil {
		panic(err)
	}

	m.disks[diskName] = disk
	return disk
}

// resolve resolves a disk instance.
func (m *Manager) resolve(name string) (contracts.Filesystem, error) {
	config := m.getConfig(name)

	driver, ok := config["driver"].(string)
	if !ok {
		return nil, fmt.Errorf("filesystem: driver not defined for disk %s", name)
	}

	if creator, ok := m.drivers[driver]; ok {
		return creator(config)
	}

	// Default drivers
	switch driver {
	case "local":
		return NewLocal(config)
	case "s3":
		return NewS3(config)
	default:
		return nil, fmt.Errorf("filesystem: driver %s not supported", driver)
	}
}

// Extend registers a custom driver creator.
func (m *Manager) Extend(driver string, creator func(config map[string]any) (contracts.Filesystem, error)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.drivers[driver] = creator
}

// getDefaultDriver gets the default driver name.
func (m *Manager) getDefaultDriver() string {
	return m.config.GetString("filesystem.default")
}

// getConfig gets the configuration for a disk.
func (m *Manager) getConfig(name string) map[string]any {
	// Need to access nested config: filesystem.disks.<name>
	// Since GetString only returns string, we might need Get("filesystem.disks." + name) which returns any
	val := m.config.Get("filesystem.disks." + name)
	if val == nil {
		return nil
	}

	// Convert map[interface{}]interface{} to map[string]any if necessary
	// But assuming the config loader (viper/etc) returns map[string]any or compatible
	if configMap, ok := val.(map[string]any); ok {
		return configMap
	}
	// Handle map[interface{}]interface{} which yaml sometimes unmarshals to
	if configMap, ok := val.(map[interface{}]interface{}); ok {
		newMap := make(map[string]any)
		for k, v := range configMap {
			newMap[fmt.Sprintf("%v", k)] = v
		}
		return newMap
	}

	return nil
}
