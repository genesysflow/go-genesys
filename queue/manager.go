package queue

import (
	"fmt"
	"sync"
)

// Manager manages queue connections.
type Manager struct {
	connections map[string]Queue
	factories   map[string]func() (Queue, error)
	defaultConn string
	mu          sync.RWMutex
}

// NewManager creates a new queue manager.
func NewManager() *Manager {
	return &Manager{
		connections: make(map[string]Queue),
		factories:   make(map[string]func() (Queue, error)),
		defaultConn: "sync",
	}
}

// Connection returns a queue connection by name (default connection when
// omitted). Lazily builds connections registered via RegisterLazy.
func (m *Manager) Connection(name ...string) (Queue, error) {
	connName := m.defaultConnName()
	if len(name) > 0 && name[0] != "" {
		connName = name[0]
	}

	m.mu.RLock()
	conn, ok := m.connections[connName]
	m.mu.RUnlock()
	if ok {
		return conn, nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if conn, ok := m.connections[connName]; ok {
		return conn, nil
	}

	if factory, ok := m.factories[connName]; ok {
		conn, err := factory()
		if err != nil {
			return nil, err
		}
		m.connections[connName] = conn
		return conn, nil
	}

	switch connName {
	case "sync":
		conn := NewSyncQueue()
		m.connections[connName] = conn
		return conn, nil
	case "memory":
		conn := NewMemoryQueue()
		m.connections[connName] = conn
		return conn, nil
	}

	return nil, fmt.Errorf("queue connection [%s] not found", connName)
}

// Register registers a pre-built queue connection.
func (m *Manager) Register(name string, queue Queue) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.connections[name] = queue
}

// RegisterLazy registers a connection factory built on first use.
func (m *Manager) RegisterLazy(name string, factory func() (Queue, error)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.factories[name] = factory
}

// SetDefaultConnection changes the default connection name.
func (m *Manager) SetDefaultConnection(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.defaultConn = name
}

// DefaultConnection returns the default connection name.
func (m *Manager) DefaultConnection() string {
	return m.defaultConnName()
}

func (m *Manager) defaultConnName() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.defaultConn
}
