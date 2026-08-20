package gate_test

import (
	"testing"

	baseauth "github.com/genesysflow/go-genesys/auth"
	"github.com/genesysflow/go-genesys/facades/gate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type actor struct{ ID int64 }

func (a *actor) GetAuthIdentifier() any  { return a.ID }
func (a *actor) GetAuthPassword() string { return "" }

func TestFacadeAbilities(t *testing.T) {
	gate.SetInstance(baseauth.NewGate())
	t.Cleanup(func() { gate.SetInstance(nil) })

	gate.Define("edit", func(user baseauth.Authenticatable, args ...any) bool {
		return user.GetAuthIdentifier() == int64(1)
	})

	owner := &actor{ID: 1}
	other := &actor{ID: 2}

	assert.True(t, gate.Allows(owner, "edit"))
	assert.True(t, gate.Denies(other, "edit"))
	require.NoError(t, gate.Authorize(owner, "edit"))
	assert.Error(t, gate.Authorize(other, "edit"))

	// Before-hook grants everything to user 2.
	gate.Before(func(user baseauth.Authenticatable, ability string, args ...any) *bool {
		if user.GetAuthIdentifier() == int64(2) {
			yes := true
			return &yes
		}
		return nil
	})
	assert.True(t, gate.Allows(other, "edit"))
	assert.NotNil(t, gate.GetInstance())
}

func TestFacadePanicsWithoutInstance(t *testing.T) {
	gate.SetInstance(nil)
	assert.Panics(t, func() { gate.Allows(&actor{}, "x") })
}
