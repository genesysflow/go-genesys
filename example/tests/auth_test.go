package tests

import (
	"testing"

	"github.com/genesysflow/go-genesys/database"
	"github.com/genesysflow/go-genesys/example/app/models"
	"github.com/genesysflow/go-genesys/example/database/factories"
	"github.com/genesysflow/go-genesys/testutil/dbtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Registration writes the user, starts a session and lands them on the
// blog - the whole scaffolded flow, through the real routes.
func TestRegisterCreatesAUserAndSignsThemIn(t *testing.T) {
	h := boot(t)

	h.submit(t, "/register", "/register", map[string]string{
		"name":                  "Ada Lovelace",
		"email":                 "ada@example.com",
		"password":              "correct horse battery",
		"password_confirmation": "correct horse battery",
	}).AssertRedirect("/posts")

	dbtest.AssertDatabaseHas(t, "users", map[string]any{"email": "ada@example.com"})

	// The session survives: a guarded page is now reachable.
	h.visit(t, "/drafts/new").AssertOK()
}

// The password is hashed, never stored as typed.
func TestRegisterHashesThePassword(t *testing.T) {
	h := boot(t)

	h.submit(t, "/register", "/register", map[string]string{
		"name":                  "Ada Lovelace",
		"email":                 "ada@example.com",
		"password":              "correct horse battery",
		"password_confirmation": "correct horse battery",
	}).AssertRedirect()

	user, err := database.Query[models.User]().Where("email", "ada@example.com").First()
	require.NoError(t, err)
	assert.NotEqual(t, "correct horse battery", user.Password)
	assert.NotEmpty(t, user.Password)
}

// A mismatched confirmation goes back to the form with the message, and
// writes nothing.
func TestRegisterRejectsAMismatchedConfirmation(t *testing.T) {
	h := boot(t)

	h.submit(t, "/register", "/register", map[string]string{
		"name":                  "Ada Lovelace",
		"email":                 "ada@example.com",
		"password":              "correct horse battery",
		"password_confirmation": "something else",
	}).AssertRedirect(testOrigin + "/register")

	dbtest.AssertDatabaseMissing(t, "users", map[string]any{"email": "ada@example.com"})

	// Following the redirect, the form carries the error and the address
	// the user typed - but never the password.
	body := h.visit(t, "/register").AssertOK().BodyString()
	assert.Contains(t, body, "ada@example.com")
	assert.NotContains(t, body, "correct horse battery")
}

// Signing in with the right password starts a session.
func TestLoginWithValidCredentials(t *testing.T) {
	h := boot(t)
	user := h.author(t)

	h.submit(t, "/login", "/login", map[string]string{
		"email":    user.Email,
		"password": factories.Password,
	}).AssertRedirect("/posts")

	h.visit(t, "/drafts/new").AssertOK()
}

// A wrong password is refused, and the message never says which half was
// wrong.
func TestLoginWithTheWrongPassword(t *testing.T) {
	h := boot(t)
	user := h.author(t)

	h.submit(t, "/login", "/login", map[string]string{
		"email":    user.Email,
		"password": "not-the-password",
	}).AssertRedirect(testOrigin + "/login")

	body := h.visit(t, "/login").AssertOK().BodyString()
	assert.Contains(t, body, "These credentials do not match our records.")

	// Still a guest: the guarded page redirects to the login form.
	h.visit(t, "/drafts/new").AssertRedirect("/login")
}

// Logging out ends the session.
func TestLogoutEndsTheSession(t *testing.T) {
	h := boot(t)
	h.signIn(t, h.author(t))

	h.visit(t, "/drafts/new").AssertOK()

	h.form(t, "/logout", nil).AssertRedirect("/")

	h.visit(t, "/drafts/new").AssertRedirect("/login")
}

// A signed-in user has no business on the login form.
func TestLoginFormRedirectsAnAuthenticatedUser(t *testing.T) {
	h := boot(t)
	h.signIn(t, h.author(t))

	h.visit(t, "/login").AssertRedirect("/posts")
}

// Writing needs a session.
func TestGuestsAreSentToTheLoginForm(t *testing.T) {
	h := boot(t)

	h.visit(t, "/drafts/new").AssertRedirect("/login")
}

// A form submitted without the CSRF token is refused, whatever it says.
func TestFormsWithoutACSRFTokenAreRefused(t *testing.T) {
	h := boot(t)
	h.signIn(t, h.author(t))

	h.tc.PostForm("/posts", map[string]string{
		"title": "No token here",
		"body":  "A body long enough to pass the rules for sure.",
	}).AssertForbidden()

	dbtest.AssertDatabaseMissing(t, "posts", map[string]any{"title": "No token here"})
}
