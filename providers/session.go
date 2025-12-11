package providers

import (
	"github.com/genesysflow/go-genesys/contracts"
	"github.com/genesysflow/go-genesys/session"
)

// SessionServiceProvider registers session services.
type SessionServiceProvider struct {
	BaseProvider

	// Config is optional session configuration.
	Config *session.Config
}

// Register registers the session services.
func (p *SessionServiceProvider) Register(app contracts.Application) error {
	p.app = app
	cfg := app.GetConfig()

	// Build session config from app config or use provided config
	sessionConfig := session.DefaultConfig()

	if p.Config != nil {
		sessionConfig = *p.Config
	} else {
		// Load from config file
		if name := cfg.GetString("session.cookie"); name != "" {
			sessionConfig.CookieName = name
		}
		if path := cfg.GetString("session.path"); path != "" {
			sessionConfig.CookiePath = path
		}
		if domain := cfg.GetString("session.domain"); domain != "" {
			sessionConfig.CookieDomain = domain
		}
		if cfg.Has("session.secure") {
			sessionConfig.CookieSecure = cfg.GetBool("session.secure")
		}
		if cfg.Has("session.http_only") {
			sessionConfig.CookieHTTPOnly = cfg.GetBool("session.http_only")
		}
		if sameSite := cfg.GetString("session.same_site"); sameSite != "" {
			sessionConfig.CookieSameSite = sameSite
		}
	}

	manager := session.NewManager(sessionConfig)
	app.BindValue("session", manager)
	app.BindValue("session.manager", manager)

	return nil
}

// Boot bootstraps the session services.
func (p *SessionServiceProvider) Boot(app contracts.Application) error {
	return nil
}

// Provides returns the services this provider registers.
func (p *SessionServiceProvider) Provides() []string {
	return []string{
		"session",
		"session.manager",
	}
}
