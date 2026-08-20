package providers

import (
	"github.com/genesysflow/go-genesys/auth"
	"github.com/genesysflow/go-genesys/contracts"
	facadeauth "github.com/genesysflow/go-genesys/facades/auth"
	facadegate "github.com/genesysflow/go-genesys/facades/gate"
)

// AuthServiceProvider registers authentication guards and the gate.
//
// Applications supply a UserProvider (usually auth.NewORMUserProvider[User]())
// and choose which guards to enable:
//
//	app.Register(&providers.AuthServiceProvider{
//	    UserProvider: auth.NewORMUserProvider[models.User](),
//	})
type AuthServiceProvider struct {
	BaseProvider

	// UserProvider retrieves users for the guards. Required for the
	// session guard; also used by the token guard when it implements
	// auth.TokenUserProvider.
	UserProvider auth.UserProvider

	// DefaultGuard is the default guard name (default "web").
	DefaultGuard string

	// DisableSessionGuard skips registering the "web" session guard.
	DisableSessionGuard bool

	// DisableTokenGuard skips registering the "api" token guard.
	DisableTokenGuard bool
}

// Register registers the auth services.
func (p *AuthServiceProvider) Register(app contracts.Application) error {
	p.app = app

	manager := auth.NewManager()
	gate := auth.NewGate()

	if p.UserProvider != nil {
		if !p.DisableSessionGuard {
			manager.RegisterGuard("web", auth.NewSessionGuard("web", p.UserProvider))
		}
		if tokenProvider, ok := p.UserProvider.(auth.TokenUserProvider); ok && !p.DisableTokenGuard {
			manager.RegisterGuard("api", auth.NewTokenGuard("api", tokenProvider))
		}
	}
	if p.DefaultGuard != "" {
		manager.SetDefaultGuard(p.DefaultGuard)
	}

	app.InstanceType(manager)
	app.InstanceType(gate)
	app.BindValue("auth", manager)
	app.BindValue("gate", gate)
	facadeauth.SetInstance(manager)
	facadegate.SetInstance(gate)

	return nil
}

// Boot bootstraps the auth services.
func (p *AuthServiceProvider) Boot(app contracts.Application) error {
	return nil
}

// Provides returns the services this provider registers.
func (p *AuthServiceProvider) Provides() []string {
	return []string{
		"auth",
		"gate",
	}
}
