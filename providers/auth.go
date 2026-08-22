package providers

import (
	"github.com/genesysflow/go-genesys/auth"
	"github.com/genesysflow/go-genesys/container"
	"github.com/genesysflow/go-genesys/contracts"
	"github.com/genesysflow/go-genesys/database"
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

	// TokensTable overrides the personal access token table name
	// (default "personal_access_tokens").
	TokensTable string
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

// Boot wires the personal access token repository, which needs a
// database connection and therefore cannot be built at register time.
func (p *AuthServiceProvider) Boot(app contracts.Application) error {
	manager, err := container.Resolve[*database.Manager](app)
	if err != nil {
		// No database configured: token authentication is simply not
		// available, which is not an error for a session-only app.
		return nil
	}

	conn := manager.Connection()
	if conn.Error() != nil {
		// No usable database, so no token repository. That is not an
		// error for an application that authenticates with sessions
		// only; anything resolving the repository will say so.
		return nil
	}

	repository := auth.NewTokenRepository(conn.Driver(), conn, p.TokensTable)
	app.InstanceType(repository)
	app.BindValue("auth.tokens", repository)

	if p.UserProvider != nil && !p.DisableTokenGuard {
		if authManager, err := container.Resolve[*auth.Manager](app); err == nil {
			authManager.RegisterGuard("sanctum", auth.NewPersonalAccessTokenGuard("sanctum", repository, p.UserProvider))
		}
	}

	return nil
}

// Provides returns the services this provider registers.
func (p *AuthServiceProvider) Provides() []string {
	return []string{
		"auth",
		"auth.tokens",
		"gate",
	}
}
