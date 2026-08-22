package tests

import (
	"net/url"
	"strings"
	"testing"

	"github.com/genesysflow/go-genesys/database"
	"github.com/genesysflow/go-genesys/example/app/models"
	"github.com/genesysflow/go-genesys/example/database/factories"
	"github.com/genesysflow/go-genesys/hash"
	"github.com/genesysflow/go-genesys/mail"
	"github.com/genesysflow/go-genesys/testutil/dbtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Asking for a reset link mails one, with a token the form can consume.
func TestRequestingAResetLink(t *testing.T) {
	h := boot(t)
	user := h.author(t)

	h.submit(t, "/forgot-password", "/forgot-password", map[string]string{
		"email": user.Email,
	}).AssertRedirect(testOrigin + "/forgot-password")

	h.mailer.AssertSentTo(t, user.Email)
	dbtest.AssertDatabaseHas(t, "password_reset_tokens", map[string]any{"email": user.Email})

	link := h.mailer.AssertSent(t, func(m *mail.Message) bool {
		return strings.Contains(m.GetHTML(), "/reset-password?token=")
	})
	require.Len(t, link, 1)
}

// An address nobody has registered gets the same answer, and no mail:
// a different response would disclose who has an account here.
func TestRequestingAResetLinkForAnUnknownAddress(t *testing.T) {
	h := boot(t)

	h.submit(t, "/forgot-password", "/forgot-password", map[string]string{
		"email": "nobody@example.com",
	}).AssertRedirect(testOrigin + "/forgot-password")

	body := h.visit(t, "/forgot-password").AssertOK().BodyString()
	assert.Contains(t, body, "If that address is registered")
}

// The whole flow: ask for a link, follow it, choose a new password, and
// sign in with it.
func TestResettingAPassword(t *testing.T) {
	h := boot(t)
	user := h.author(t)

	h.submit(t, "/forgot-password", "/forgot-password", map[string]string{"email": user.Email})

	token := h.resetToken(t)
	require.NotEmpty(t, token)

	h.submit(t, "/reset-password?token="+url.QueryEscape(token)+"&email="+url.QueryEscape(user.Email),
		"/reset-password", map[string]string{
			"token":                 token,
			"email":                 user.Email,
			"password":              "a whole new password",
			"password_confirmation": "a whole new password",
		}).AssertRedirect("/login")

	// The password really changed.
	fresh, err := database.Find[models.User](user.ID)
	require.NoError(t, err)
	assert.NoError(t, hash.Check("a whole new password", fresh.Password))
	assert.Error(t, hash.Check(factories.Password, fresh.Password))

	// And the new one signs in.
	h.tc.FlushCookies()
	h.submit(t, "/login", "/login", map[string]string{
		"email":    user.Email,
		"password": "a whole new password",
	}).AssertRedirect("/posts")
}

// A token that was never issued changes nothing.
func TestResettingWithAnInvalidToken(t *testing.T) {
	h := boot(t)
	user := h.author(t)

	h.submit(t, "/reset-password", "/reset-password", map[string]string{
		"token":                 "not-a-real-token",
		"email":                 user.Email,
		"password":              "a whole new password",
		"password_confirmation": "a whole new password",
	}).AssertRedirect(testOrigin + "/reset-password")

	fresh, err := database.Find[models.User](user.ID)
	require.NoError(t, err)
	assert.NoError(t, hash.Check(factories.Password, fresh.Password), "the password should be unchanged")
}

// A token is spent once. Reusing it does not change the password again.
func TestAResetTokenIsSpentOnce(t *testing.T) {
	h := boot(t)
	user := h.author(t)

	h.submit(t, "/forgot-password", "/forgot-password", map[string]string{"email": user.Email})
	token := h.resetToken(t)

	fields := map[string]string{
		"token":                 token,
		"email":                 user.Email,
		"password":              "a whole new password",
		"password_confirmation": "a whole new password",
	}
	h.submit(t, "/reset-password", "/reset-password", fields).AssertRedirect("/login")

	second := map[string]string{
		"token":                 token,
		"email":                 user.Email,
		"password":              "yet another password",
		"password_confirmation": "yet another password",
	}
	h.submit(t, "/reset-password", "/reset-password", second).
		AssertRedirect(testOrigin + "/reset-password")

	fresh, err := database.Find[models.User](user.ID)
	require.NoError(t, err)
	assert.NoError(t, hash.Check("a whole new password", fresh.Password))
}

// Registration greets the new author, as a mailable rather than a
// message assembled by hand.
func TestRegistrationSendsAWelcome(t *testing.T) {
	h := boot(t)

	h.submit(t, "/register", "/register", map[string]string{
		"name":                  "Ada Lovelace",
		"email":                 "ada@example.com",
		"password":              "correct horse battery",
		"password_confirmation": "correct horse battery",
	}).AssertRedirect("/posts")

	h.mailer.AssertSentTo(t, "ada@example.com")
	h.mailer.AssertSent(t, func(m *mail.Message) bool {
		return strings.Contains(m.GetSubject(), "Welcome to the blog")
	})
}
