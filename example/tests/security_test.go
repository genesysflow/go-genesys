package tests

import (
	"fmt"
	nethttp "net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/genesysflow/go-genesys/database"
	"github.com/genesysflow/go-genesys/example/app/models"
	"github.com/genesysflow/go-genesys/example/database/factories"
	genhttp "github.com/genesysflow/go-genesys/http"
	"github.com/genesysflow/go-genesys/testutil/dbtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- access control -------------------------------------------------

// Knowing another author's URL is not permission to use it.
func TestAnAuthorCannotEditAnotherAuthorsPost(t *testing.T) {
	h := boot(t)
	victim := h.author(t)
	post := h.post(t, victim)

	h.signIn(t, h.author(t))

	h.visit(t, "/posts/"+post.Slug+"/edit").AssertForbidden()

	h.visit(t, "/posts").AssertOK()
	h.tc.Do(h.formRequest(t, "PUT", "/posts/"+post.Slug, map[string]string{
		"title": "Defaced by a stranger",
		"slug":  post.Slug,
		"body":  "A body long enough to satisfy the rules of the form.",
	})).AssertForbidden()

	dbtest.AssertDatabaseHas(t, "posts", map[string]any{"id": post.ID, "title": post.Title})
}

// Nor to delete it.
func TestAnAuthorCannotDeleteAnotherAuthorsPost(t *testing.T) {
	h := boot(t)
	post := h.post(t, h.author(t))

	h.signIn(t, h.author(t))
	h.visit(t, "/posts").AssertOK()
	h.tc.Do(h.formRequest(t, "DELETE", "/posts/"+post.Slug, nil)).AssertForbidden()

	dbtest.AssertDatabaseHas(t, "posts", map[string]any{"id": post.ID, "deleted_at": nil})
}

// A form field nobody asked for must not reach the model. author_id
// decides who owns a post, and it is set from the session, never from
// the request.
func TestAPostCannotBeAttributedToSomeoneElse(t *testing.T) {
	h := boot(t)
	victim := h.author(t)
	attacker := h.signIn(t, h.author(t))

	h.submit(t, "/drafts/new", "/posts", map[string]string{
		"title":     "Planted under another name",
		"body":      "A body long enough to satisfy the rules of the form.",
		"author_id": itoa(victim.ID),
		"id":        "1",
	}).AssertRedirect()

	post, err := database.Query[models.Post]().Where("slug", "planted-under-another-name").First()
	require.NoError(t, err)
	assert.Equal(t, attacker.ID, post.AuthorID,
		"author_id must come from the session, not the form")
}

// The same through the JSON API, where a client controls the whole body.
func TestTheAPICannotAttributeAPostToSomeoneElse(t *testing.T) {
	h := boot(t)
	victim := h.author(t)
	attacker := h.author(t)

	h.api(t, h.token(t, attacker, "posts:write")).
		Post("/api/blog/posts", map[string]any{
			"title":     "Planted through the API",
			"body":      "A body long enough to satisfy the rules of the form.",
			"author_id": victim.ID,
			"id":        1,
		}).
		AssertCreated()

	post, err := database.Query[models.Post]().Where("slug", "planted-through-the-api").First()
	require.NoError(t, err)
	assert.Equal(t, attacker.ID, post.AuthorID)
}

// A published_at in the payload must not publish a post: publishing is
// an editor's decision, and the form request does not bind the column.
func TestAPostCannotPublishItself(t *testing.T) {
	h := boot(t)
	h.signIn(t, h.author(t))

	h.submit(t, "/drafts/new", "/posts", map[string]string{
		"title":        "Self published",
		"body":         "A body long enough to satisfy the rules of the form.",
		"published_at": "2020-01-01T00:00:00Z",
	}).AssertRedirect()

	post, err := database.Query[models.Post]().Where("slug", "self-published").First()
	require.NoError(t, err)
	assert.Nil(t, post.PublishedAt, "a draft must not publish itself")
}

// Registration must not let a caller choose their own role.
func TestRegistrationCannotGrantEditor(t *testing.T) {
	h := boot(t)

	h.submit(t, "/register", "/register", map[string]string{
		"name":                  "Ada Lovelace",
		"email":                 "ada@example.com",
		"password":              "correct horse battery",
		"password_confirmation": "correct horse battery",
		"role":                  "editor",
	}).AssertRedirect()

	user, err := database.Query[models.User]().Where("email", "ada@example.com").First()
	require.NoError(t, err)
	assert.Equal(t, "author", user.Role, "a caller must not be able to choose their role")
}

// An author cannot publish, whatever they send.
func TestAnAuthorCannotPublish(t *testing.T) {
	h := boot(t)
	author := h.signIn(t, h.author(t))
	post := h.post(t, author)

	h.visit(t, "/posts/"+post.Slug).AssertOK()
	h.form(t, "/posts/"+post.Slug+"/publish", nil).AssertForbidden()

	fresh, err := database.Find[models.Post](post.ID)
	require.NoError(t, err)
	assert.Nil(t, fresh.PublishedAt)
}

// A token is bound to the user it was issued to: it cannot be used to
// write as somebody else.
func TestATokenActsOnlyForItsOwner(t *testing.T) {
	h := boot(t)
	owner := h.author(t)
	other := h.author(t)

	h.api(t, h.token(t, owner, "profile:read")).
		Get("/api/blog/me").
		AssertOK().
		AssertJsonPath("data.user.email", owner.Email)

	assert.NotEqual(t, owner.ID, other.ID)
}

// itoa keeps the form helpers stringly-typed without importing strconv
// into every test.
func itoa(v int64) string {
	if v == 0 {
		return "0"
	}
	digits := ""
	for v > 0 {
		digits = string(rune('0'+v%10)) + digits
		v /= 10
	}
	return digits
}

// --- session and authentication -------------------------------------

// A session id fixed before the login must not survive it, or an
// attacker who planted one rides the session the victim just
// authenticated.
func TestLoginRotatesTheSessionID(t *testing.T) {
	h := boot(t)
	user := h.author(t)

	h.visit(t, "/login").AssertOK()
	before := h.tc.Cookie("genesys_session")
	require.NotEmpty(t, before)

	h.form(t, "/login", map[string]string{
		"email":    user.Email,
		"password": factories.Password,
	}).AssertRedirect()

	after := h.tc.Cookie("genesys_session")
	assert.NotEqual(t, before, after, "the session id must change on login")
}

// And on logout, so the id a shared machine keeps is worthless.
func TestLogoutRotatesTheSessionID(t *testing.T) {
	h := boot(t)
	h.signIn(t, h.author(t))

	before := h.tc.Cookie("genesys_session")
	h.form(t, "/logout", nil).AssertRedirect()

	assert.NotEqual(t, before, h.tc.Cookie("genesys_session"))
}

// A session cookie must not be readable by scripts.
func TestTheSessionCookieIsHTTPOnly(t *testing.T) {
	h := boot(t)

	response := h.tc.Get("/posts").AssertOK()

	cookie := response.Cookie("genesys_session")
	require.NotNil(t, cookie, "the response should set a session cookie")
	assert.True(t, cookie.HttpOnly, "a session cookie must not be readable by scripts")
	assert.Equal(t, nethttp.SameSiteLaxMode, cookie.SameSite,
		"SameSite keeps the cookie off cross-site requests")
}

// Whatever the login form says, the response must not tell an attacker
// which addresses are registered.
func TestLoginDoesNotDiscloseWhoHasAnAccount(t *testing.T) {
	h := boot(t)
	user := h.author(t)

	h.submit(t, "/login", "/login", map[string]string{
		"email":    user.Email,
		"password": "wrong",
	})
	known := h.visit(t, "/login").AssertOK().BodyString()

	h.tc.FlushCookies()
	h.submit(t, "/login", "/login", map[string]string{
		"email":    "nobody@example.com",
		"password": "wrong",
	})
	unknown := h.visit(t, "/login").AssertOK().BodyString()

	assert.Contains(t, known, "These credentials do not match our records.")
	assert.Contains(t, unknown, "These credentials do not match our records.")
}

// The password is never echoed back into the form, however the request
// failed.
func TestAFailedLoginDoesNotFlashThePassword(t *testing.T) {
	h := boot(t)

	h.submit(t, "/login", "/login", map[string]string{
		"email":    "ada@example.com",
		"password": "hunter2-the-secret",
	})

	body := h.visit(t, "/login").AssertOK().BodyString()
	assert.Contains(t, body, "ada@example.com", "the address is repopulated")
	assert.NotContains(t, body, "hunter2-the-secret", "the password never is")
}

// A token's plaintext exists once. The database holds a hash, so a
// leaked table hands over nothing usable.
func TestTokensAreStoredHashed(t *testing.T) {
	h := boot(t)
	plaintext := h.token(t, h.author(t), "profile:read")

	_, secret, found := strings.Cut(plaintext, "|")
	require.True(t, found)

	dbtest.AssertDatabaseMissing(t, "personal_access_tokens", map[string]any{"token": secret})
	dbtest.AssertDatabaseMissing(t, "personal_access_tokens", map[string]any{"token": plaintext})
}

// A token belonging to one user must not be usable by editing its id
// half: the secret is what is checked.
func TestATokenCannotBeForgedByChangingItsID(t *testing.T) {
	h := boot(t)
	owner := h.author(t)
	plaintext := h.token(t, owner, "profile:read")

	_, secret, _ := strings.Cut(plaintext, "|")

	for _, forged := range []string{
		"999|" + secret,
		"1|" + secret + "x",
		"1|",
		"|" + secret,
		secret,
	} {
		h.api(t, forged).Get("/api/blog/me").AssertUnauthorized()
	}
}

// A reset token is not guessable from the row: only its hash is stored.
func TestResetTokensAreStoredHashed(t *testing.T) {
	h := boot(t)
	user := h.author(t)

	h.submit(t, "/forgot-password", "/forgot-password", map[string]string{"email": user.Email})
	token := h.resetToken(t)

	dbtest.AssertDatabaseMissing(t, "password_reset_tokens", map[string]any{"token": token})
	dbtest.AssertDatabaseHas(t, "password_reset_tokens", map[string]any{"email": user.Email})
}

// A password must not be guessable at machine speed: the login endpoint
// is rate-limited, and says so with a 429 and a Retry-After.
func TestLoginIsThrottled(t *testing.T) {
	h := boot(t)
	user := h.author(t)

	var lastStatus int
	for i := 0; i < 20; i++ {
		lastStatus = h.submit(t, "/login", "/login", map[string]string{
			"email":    user.Email,
			"password": "wrong",
		}).Status()

		if lastStatus == 429 {
			break
		}
	}

	assert.Equal(t, 429, lastStatus, "repeated failed logins should be refused")
}

// The limit is per address, so one attacker hammering one account does
// not lock everybody else out.
func TestThrottlingIsPerAccount(t *testing.T) {
	h := boot(t)
	victim := h.author(t)
	bystander := h.author(t)

	for i := 0; i < 20; i++ {
		if h.submit(t, "/login", "/login", map[string]string{
			"email":    victim.Email,
			"password": "wrong",
		}).Status() == 429 {
			break
		}
	}

	h.tc.FlushCookies()
	h.submit(t, "/login", "/login", map[string]string{
		"email":    bystander.Email,
		"password": factories.Password,
	}).AssertRedirect("/posts")
}

// --- disclosure -----------------------------------------------------

// A password hash must not leave the application, whatever renders the
// user - the API, a relation, a nested payload.
func TestThePasswordHashNeverLeaves(t *testing.T) {
	h := boot(t)
	author := h.author(t)
	h.published(t, author)

	token := h.token(t, author, "profile:read")

	h.api(t, token).Get("/api/blog/me").AssertOK().
		AssertJsonMissing("data.user.password")

	// And through the post index, which eager-loads the author.
	body := h.api(t, token).Get("/api/blog/posts").AssertOK().BodyString()
	assert.NotContains(t, body, author.Password)
	assert.NotContains(t, body, "password")
}

// An error page must not hand a client the internals - credentials,
// hostnames, file paths - on a production box.
func TestErrorsDoNotLeakInternalsInProduction(t *testing.T) {
	h := bootProduction(t)

	h.kernel.GET("/_test/boom", func(ctx *genhttp.Context) error {
		return fmt.Errorf("connection to postgres://user:hunter2@10.0.0.5/app failed")
	})

	body := h.tc.Get("/_test/boom").AssertStatus(500).BodyString()

	assert.NotContains(t, body, "hunter2")
	assert.NotContains(t, body, "10.0.0.5")
	assert.NotContains(t, body, "/home/")
}

// The development panel serves request paths and SQL, which is exactly
// what must not be reachable on a production box.
func TestTheDevelopmentPanelIsNotMountedInProduction(t *testing.T) {
	h := bootProduction(t)

	h.tc.Get("/_genesys").AssertNotFound()
}

// A draft's contents must not leak through the 403 that hides it.
func TestAForbiddenDraftDoesNotLeakItsBody(t *testing.T) {
	h := boot(t)
	draft := h.post(t, h.author(t))

	h.signIn(t, h.author(t))
	body := h.visit(t, "/posts/"+draft.Slug).AssertForbidden().BodyString()

	assert.NotContains(t, body, draft.Body)
	assert.NotContains(t, body, draft.Title)
}

// --- traversal ------------------------------------------------------

// A route parameter is data. A slug that walks out of the table, or out
// of the views directory, must find nothing rather than something.
func TestRouteParametersCannotTraverse(t *testing.T) {
	h := boot(t)

	for _, slug := range []string{
		"../../../etc/passwd",
		"..%2F..%2Fetc%2Fpasswd",
		"%2e%2e%2f%2e%2e%2fetc%2fpasswd",
	} {
		status := h.tc.Get("/posts/" + slug).Status()
		assert.Contains(t, []int{404, 400, 301, 302}, status,
			"a traversing slug returned %d for %q", status, slug)
	}
}

// A slug carrying SQL is a slug, not SQL.
func TestASlugCarryingSQLIsJustAMiss(t *testing.T) {
	h := boot(t)
	post := h.published(t, h.author(t))

	h.tc.Get("/posts/" + url.QueryEscape("' OR '1'='1")).AssertNotFound()

	// The table is intact.
	dbtest.AssertDatabaseHas(t, "posts", map[string]any{"id": post.ID})
	dbtest.AssertDatabaseCount(t, "posts", 1)
}

// --- response headers -----------------------------------------------

// The headers that cost nothing and close whole classes of attack are
// on: no MIME sniffing, no framing, no referrer leaking to other sites.
func TestSecurityHeadersArePresent(t *testing.T) {
	h := boot(t)

	response := h.tc.Get("/posts").AssertOK()

	assert.Equal(t, "nosniff", response.Header("X-Content-Type-Options"))
	assert.Equal(t, "SAMEORIGIN", response.Header("X-Frame-Options"))
	assert.Equal(t, "strict-origin-when-cross-origin", response.Header("Referrer-Policy"))
}

// A cookie-authenticated page must not be readable by any origin that
// asks: CORS belongs on the token API, not on the session-backed site.
func TestTheHTMLSiteIsNotCORSOpen(t *testing.T) {
	h := boot(t)

	response := h.tc.Do(genhttp.Get("/posts").WithHeader("Origin", "https://evil.example.com"))
	assert.Empty(t, response.Header("Access-Control-Allow-Origin"),
		"the HTML site should not advertise itself as cross-origin readable")
}

// The token API is meant to be called from elsewhere, so it does say so.
func TestTheAPIAllowsCrossOriginCallers(t *testing.T) {
	h := boot(t)
	token := h.token(t, h.author(t), "profile:read")

	response := h.api(t, token).Do(
		genhttp.Get("/api/blog/me").WithHeader("Origin", "https://app.example.com"))

	response.AssertOK()
	assert.NotEmpty(t, response.Header("Access-Control-Allow-Origin"))
}
