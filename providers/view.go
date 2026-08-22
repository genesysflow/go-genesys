package providers

import (
	"github.com/genesysflow/go-genesys/container"
	"github.com/genesysflow/go-genesys/contracts"
	facadeview "github.com/genesysflow/go-genesys/facades/view"
	"github.com/genesysflow/go-genesys/http"
	"github.com/genesysflow/go-genesys/lang"
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
			cfg.Path = basePathFor(app, path)
		}
		if appCfg.Has("view.reload") {
			cfg.Reload = appCfg.GetBool("view.reload")
		} else {
			// Reload templates automatically outside production.
			cfg.Reload = appCfg.GetString("app.env") != "production"
		}
	}

	manager := view.NewManager(cfg)
	wireViewHelpers(app, manager)
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

// wireViewHelpers connects the request-independent template helpers to
// the application. Each one resolves its dependency when the template
// runs, not now, so the router and the translator may be registered
// before or after the view provider.
func wireViewHelpers(app contracts.Application, manager *view.Manager) {
	if cfg := app.GetConfig(); cfg != nil {
		manager.SetBaseURL(cfg.GetString("app.url"))
		manager.SetConfigResolver(cfg.Get)
	}

	manager.SetURLGenerator(func(name string, params ...map[string]any) string {
		router, err := container.Resolve[*http.Router](app, "router")
		if err != nil {
			return ""
		}
		return router.URL(name, params...)
	})

	manager.SetTranslator(func(key string, replacements ...map[string]string) string {
		translator, err := container.Resolve[*lang.Translator](app, "translator")
		if err != nil {
			return key
		}
		return translator.Trans(key, replacements...)
	})
}
