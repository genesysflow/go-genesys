package tests

import (
	"testing"

	"github.com/genesysflow/go-genesys/auth"
	"github.com/genesysflow/go-genesys/example/app/models"
	genhttp "github.com/genesysflow/go-genesys/http"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// probeCurrentUser mounts a page that reports who is reading it. It is
// the whole point of models.CurrentUser: one call, no type assertion,
// and nothing to panic over when nobody is signed in.
func (h *harness) probeCurrentUser(t *testing.T) {
	t.Helper()

	h.kernel.GET("/_test/current-user", func(ctx *genhttp.Context) error {
		user := models.CurrentUser(ctx)
		if user == nil {
			return ctx.String("guest")
		}
		return ctx.String(user.Email)
	})
}

// A guest gets nil rather than a panic, which is what a public page
// needs to decide whether to show an edit link.
func TestCurrentUserIsNilForAGuest(t *testing.T) {
	h := boot(t)
	h.probeCurrentUser(t)

	h.visit(t, "/_test/current-user").AssertOK().AssertSee("guest")
}

func TestCurrentUserIsTheSignedInUser(t *testing.T) {
	h := boot(t)
	h.probeCurrentUser(t)

	user := h.signIn(t, h.author(t))

	h.visit(t, "/_test/current-user").AssertOK().AssertSee(user.Email)
}

// otherUser is an authenticatable this application does not own - what
// a second guard, or another package's fixture, hands a gate.
type otherUser struct{}

func (o *otherUser) GetAuthIdentifier() any  { return 1 }
func (o *otherUser) GetAuthPassword() string { return "" }

var _ auth.Authenticatable = (*otherUser)(nil)

func TestAsUserNarrowsToTheApplicationModel(t *testing.T) {
	account := &models.User{Email: "ada@example.com", Role: "editor"}

	user, ok := models.AsUser(account)
	require.True(t, ok)
	assert.Equal(t, "ada@example.com", user.Email)
	assert.True(t, user.IsEditor())
}

func TestAsUserRejectsGuestsAndForeignModels(t *testing.T) {
	user, ok := models.AsUser(nil)
	assert.False(t, ok, "a guest is nobody")
	assert.Nil(t, user)

	user, ok = models.AsUser(&otherUser{})
	assert.False(t, ok, "another package's user is not ours")
	assert.Nil(t, user)

	// A typed nil satisfies the interface but is still nobody, and the
	// caller must not have to remember that.
	user, ok = models.AsUser((*models.User)(nil))
	assert.False(t, ok, "a typed nil is still nobody")
	assert.Nil(t, user)
}
