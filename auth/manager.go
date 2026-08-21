package auth

import (
	"fmt"
	"sync"
)

// Manager holds the application's named guards.
type Manager struct {
	guards       map[string]Guard
	defaultGuard string
	mu           sync.RWMutex
}

// NewManager creates an auth manager. The first registered guard becomes
// the default unless SetDefaultGuard is called.
func NewManager() *Manager {
	return &Manager{guards: make(map[string]Guard)}
}

// RegisterGuard registers a named guard.
func (m *Manager) RegisterGuard(name string, guard Guard) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.guards[name] = guard
	if m.defaultGuard == "" {
		m.defaultGuard = name
	}
}

// SetDefaultGuard sets the default guard name.
func (m *Manager) SetDefaultGuard(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.defaultGuard = name
}

// Guard returns a guard by name (default guard when omitted).
func (m *Manager) Guard(name ...string) (Guard, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	guardName := m.defaultGuard
	if len(name) > 0 && name[0] != "" {
		guardName = name[0]
	}
	guard, ok := m.guards[guardName]
	if !ok {
		return nil, fmt.Errorf("auth: guard [%s] is not registered", guardName)
	}
	return guard, nil
}

// MustGuard returns a guard by name or panics.
func (m *Manager) MustGuard(name ...string) Guard {
	guard, err := m.Guard(name...)
	if err != nil {
		panic(err)
	}
	return guard
}
