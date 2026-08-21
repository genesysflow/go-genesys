// Package route provides a static facade for URL generation from named
// routes.
package route

import (
	"sync"

	"github.com/genesysflow/go-genesys/http"
)

var (
	instance *http.Router
	mu       sync.RWMutex
)

// SetInstance sets the router instance.
// This is called during application bootstrap.
func SetInstance(router *http.Router) {
	mu.Lock()
	defer mu.Unlock()
	instance = router
}

// GetInstance returns the router instance.
func GetInstance() *http.Router {
	mu.RLock()
	defer mu.RUnlock()
	return instance
}

func router() *http.Router {
	mu.RLock()
	defer mu.RUnlock()
	if instance == nil {
		panic("route: facade not initialised - register the RouteServiceProvider")
	}
	return instance
}

// URL generates a URL for a named route:
//
//	route.URL("users.show", map[string]any{"id": 42})
func URL(name string, params ...map[string]any) string {
	return router().URL(name, params...)
}

// Has reports whether a named route exists.
func Has(name string) bool {
	return router().NamedRoute(name) != nil
}
