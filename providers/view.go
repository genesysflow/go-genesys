package providers

import (
	"github.com/genesysflow/go-genesys/contracts"
	facadeview "github.com/genesysflow/go-genesys/facades/view"
	"github.com/genesysflow/go-genesys/view"
)

// ViewServiceProvider registers the view layer.
//
// Configuration (config/view.yaml):
//
//	path: resources/views
//	reload: false
type ViewServiceProvider struct {
	BaseProvider

	// Config is optional view configuration.
	Config *view.Config
}

// Register registers the view services.
func (p *ViewServiceProvider) Register(app contracts.Application) error {
	p.app = app
	return nil
}

// Boot builds the view manager from configuration.
func (p *ViewServiceProvider) Boot(app contracts.Application) error {
	cfg := view.Config{}
	if p.Config != nil {
		cfg = *p.Config
	} else {
		appCfg := app.GetConfig()
		if path := appCfg.GetString("view.path"); path != "" {
			cfg.Path = path
		}
		if appCfg.Has("view.reload") {
			cfg.Reload = appCfg.GetBool("view.reload")
		} else {
			// Reload templates automatically outside production.
			cfg.Reload = appCfg.GetString("app.env") != "production"
		}
	}

	manager := view.NewManager(cfg)
	app.InstanceType(manager)
	app.BindValue("view", manager)
	facadeview.SetInstance(manager)

	return nil
}

// Provides returns the services this provider registers.
func (p *ViewServiceProvider) Provides() []string {
	return []string{
		"view",
	}
}
