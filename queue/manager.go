package queue

import (
	"fmt"
	"sync"
)

// Manager manages queue connections.
type Manager struct {
	connections map[string]Queue
	defaultConn string
	mu          sync.RWMutex
}

// NewManager creates a new queue manager.
func NewManager() *Manager {
	return &Manager{
		connections: make(map[string]Queue),
		defaultConn: "sync",
	}
}

// Connection returns a queue connection by name.
func (m *Manager) Connection(name ...string) (Queue, error) {
	connName := m.defaultConn
	if len(name) > 0 && name[0] != "" {
		connName = name[0]
	}

	m.mu.RLock()
	conn, ok := m.connections[connName]
	m.mu.RUnlock()

	if ok {
		return conn, nil
	}

	// Create connection if not exists
	if connName == "sync" {
		m.mu.Lock()
		defer m.mu.Unlock()
		if conn, ok := m.connections[connName]; ok {
			return conn, nil
		}
		conn = NewSyncQueue()
		m.connections[connName] = conn
		return conn, nil
	}

	return nil, fmt.Errorf("queue connection [%s] not found", connName)
}

// Register registers a queue connection.
func (m *Manager) Register(name string, queue Queue) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.connections[name] = queue
}
