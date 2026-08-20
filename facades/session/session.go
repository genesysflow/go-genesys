// Package session provides a static facade for the session manager.
package session

import (
	"sync"

	basesession "github.com/genesysflow/go-genesys/session"
	"github.com/gofiber/fiber/v2"
)

var (
	instance *basesession.Manager
	mu       sync.RWMutex
)

// SetInstance sets the session manager instance.
// This is called during application bootstrap.
func SetInstance(manager *basesession.Manager) {
	mu.Lock()
	defer mu.Unlock()
	instance = manager
}

// GetInstance returns the session manager instance.
func GetInstance() *basesession.Manager {
	mu.RLock()
	defer mu.RUnlock()
	return instance
}

func manager() *basesession.Manager {
	mu.RLock()
	defer mu.RUnlock()
	if instance == nil {
		panic("session: facade not initialised - register the SessionServiceProvider")
	}
	return instance
}

// Middleware returns the session middleware for the kernel.
func Middleware() fiber.Handler {
	return manager().Middleware()
}

// FromContext returns the request's session (nil when the middleware is
// not registered).
func FromContext(c *fiber.Ctx) *basesession.Session {
	return basesession.GetFromContext(c)
}
