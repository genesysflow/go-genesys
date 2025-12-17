package providers

import (
	"github.com/genesysflow/go-genesys/cache"
	"github.com/genesysflow/go-genesys/contracts"
)

// CacheServiceProvider registers the cache services.
type CacheServiceProvider struct {
	BaseProvider
}

// Register registers the cache services.
func (p *CacheServiceProvider) Register(app contracts.Application) error {
	p.app = app

	manager := cache.NewManager()
	app.InstanceType(manager)
	app.BindValue("cache", manager)

	return nil
}

// Boot bootstraps the cache services.
func (p *CacheServiceProvider) Boot(app contracts.Application) error {
	return nil
}

// Provides returns the services this provider registers.
func (p *CacheServiceProvider) Provides() []string {
	return []string{
		"cache",
	}
}
