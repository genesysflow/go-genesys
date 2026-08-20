// Package view provides a static facade for the view manager.
package view

import (
	"sync"

	baseview "github.com/genesysflow/go-genesys/view"
)

var (
	instance *baseview.Manager
	mu       sync.RWMutex
)

// SetInstance sets the view manager instance.
// This is called during application bootstrap.
func SetInstance(manager *baseview.Manager) {
	mu.Lock()
	defer mu.Unlock()
	instance = manager
}

// GetInstance returns the view manager instance.
func GetInstance() *baseview.Manager {
	mu.RLock()
	defer mu.RUnlock()
	return instance
}

func manager() *baseview.Manager {
	mu.RLock()
	defer mu.RUnlock()
	if instance == nil {
		panic("view: facade not initialised - register the ViewServiceProvider")
	}
	return instance
}

// Render renders a view to a string.
func Render(name string, data map[string]any) (string, error) {
	return manager().RenderString(name, data)
}

// Exists reports whether a view is available.
func Exists(name string) bool {
	return manager().Exists(name)
}

// Share makes a value available to every template.
func Share(key string, value any) {
	manager().Share(key, value)
}
