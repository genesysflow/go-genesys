package auth_test

import (
	"testing"

	baseauth "github.com/genesysflow/go-genesys/auth"
	"github.com/genesysflow/go-genesys/facades/auth"
	"github.com/stretchr/testify/assert"
)

type user struct{ ID int64 }

func (u *user) GetAuthIdentifier() any  { return u.ID }
func (u *user) GetAuthPassword() string { return "" }

type staticProvider struct{ user *user }

func (p *staticProvider) RetrieveByID(id any) (baseauth.Authenticatable, error) {
	return p.user, nil
}
func (p *staticProvider) RetrieveByCredentials(credentials map[string]any) (baseauth.Authenticatable, error) {
	return p.user, nil
}
func (p *staticProvider) RetrieveByToken(token string) (baseauth.Authenticatable, error) {
	return p.user, nil
}

func TestFacadeGuardResolution(t *testing.T) {
	manager := baseauth.NewManager()
	provider := &staticProvider{user: &user{ID: 1}}
	manager.RegisterGuard("web", baseauth.NewSessionGuard("web", provider))
	manager.RegisterGuard("api", baseauth.NewTokenGuard("api", provider))

	auth.SetInstance(manager)
	t.Cleanup(func() { auth.SetInstance(nil) })

	assert.NotNil(t, auth.Guard()) // default = first registered ("web")
	assert.NotNil(t, auth.Guard("api"))
	assert.Panics(t, func() { auth.Guard("missing") })
	assert.NotNil(t, auth.GetInstance())
}

func TestStatefulHelpersRejectTokenGuard(t *testing.T) {
	manager := baseauth.NewManager()
	provider := &staticProvider{user: &user{ID: 1}}
	manager.RegisterGuard("api", baseauth.NewTokenGuard("api", provider))

	auth.SetInstance(manager)
	t.Cleanup(func() { auth.SetInstance(nil) })

	// The default guard is the token guard, which is not stateful.
	assert.Panics(t, func() { auth.Attempt(nil, nil) })
	assert.Panics(t, func() { auth.Login(nil, &user{}) })
	assert.Panics(t, func() { auth.Logout(nil) })
}

func TestFacadePanicsWithoutInstance(t *testing.T) {
	auth.SetInstance(nil)
	assert.Panics(t, func() { auth.Guard() })
}
