package providers

import (
	"github.com/genesysflow/go-genesys/cache"
	facadecache "github.com/genesysflow/go-genesys/facades/cache"

	"github.com/genesysflow/go-genesys/contracts"
)

// CacheServiceProvider registers the cache services.
type CacheServiceProvider struct {
	BaseProvider

	// Config is optional cache configuration.
	// If nil, configuration is loaded from config/cache.yaml.
	Config *cache.Config

	manager *cache.Manager
}

// Register registers the cache services.
func (p *CacheServiceProvider) Register(app contracts.Application) error {
	p.app = app

	manager := cache.NewManager()
	p.manager = manager
	app.InstanceType(manager)
	app.BindValue("cache", manager)
	facadecache.SetInstance(manager)

	return nil
}

// Boot applies cache configuration from config/cache.yaml.
func (p *CacheServiceProvider) Boot(app contracts.Application) error {
	cfg := p.Config
	if cfg == nil {
		cfg = &cache.Config{Stores: make(map[string]cache.StoreConfig)}
		appCfg := app.GetConfig()
		if def := appCfg.GetString("cache.default"); def != "" {
			cfg.Default = def
		}
		if stores, ok := appCfg.Get("cache.stores").(map[string]any); ok {
			for name, raw := range stores {
				storeCfg := cache.StoreConfig{}
				if details, ok := raw.(map[string]any); ok {
					if driver, ok := details["driver"].(string); ok {
						storeCfg.Driver = driver
					}
					if path, ok := details["path"].(string); ok {
						storeCfg.Path = path
					}
					if addr, ok := details["addr"].(string); ok {
						storeCfg.Addr = addr
					}
					if password, ok := details["password"].(string); ok {
						storeCfg.Password = password
					}
					if db, ok := details["db"].(int); ok {
						storeCfg.DB = db
					}
					if prefix, ok := details["prefix"].(string); ok {
						storeCfg.Prefix = prefix
					}
				}
				cfg.Stores[name] = storeCfg
			}
		}
	}

	p.manager.Configure(*cfg)

	return nil
}

// Provides returns the services this provider registers.
func (p *CacheServiceProvider) Provides() []string {
	return []string{
		"cache",
	}
}
