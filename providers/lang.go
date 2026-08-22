package providers

import (
	"github.com/genesysflow/go-genesys/contracts"
	facadelang "github.com/genesysflow/go-genesys/facades/lang"
	"github.com/genesysflow/go-genesys/lang"
)

// LangServiceProvider registers the translator.
//
// Configuration: app.locale, app.fallback_locale, lang.path.
type LangServiceProvider struct {
	BaseProvider

	// Config is optional translator configuration.
	Config *lang.Config
}

// Register registers the translator.
func (p *LangServiceProvider) Register(app contracts.Application) error {
	p.app = app
	return nil
}

// Boot builds the translator from configuration.
func (p *LangServiceProvider) Boot(app contracts.Application) error {
	cfg := lang.Config{}
	if p.Config != nil {
		cfg = *p.Config
	} else {
		appCfg := app.GetConfig()
		cfg.Locale = appCfg.GetString("app.locale")
		cfg.Fallback = appCfg.GetString("app.fallback_locale")
		cfg.Path = basePathFor(app, appCfg.GetString("lang.path"))
	}

	translator := lang.New(cfg)
	app.InstanceType(translator)
	app.BindValue("translator", translator)
	facadelang.SetInstance(translator)

	return nil
}

// Provides returns the services this provider registers.
func (p *LangServiceProvider) Provides() []string {
	return []string{
		"translator",
	}
}
