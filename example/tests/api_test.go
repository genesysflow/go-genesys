package tests

import (
	"testing"

	"github.com/genesysflow/go-genesys/example/app/models"
	"github.com/genesysflow/go-genesys/testutil/dbtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A token is issued from the session-authenticated site, and is the only
// time the plaintext exists.
func TestIssuingAnAPIToken(t *testing.T) {
	h := boot(t)
	author := h.signIn(t, h.author(t))

	response := h.form(t, "/api-tokens", map[string]string{
		"name":      "reader",
		"abilities": "profile:read",
	}).AssertCreated()

	var payload map[string]any
	require.NoError(t, response.JSON(&payload))

	plaintext, _ := payload["token"].(string)
	require.NotEmpty(t, plaintext)

	// The database holds a hash, never the token itself.
	dbtest.AssertDatabaseMissing(t, "personal_access_tokens", map[string]any{"token": plaintext})
	dbtest.AssertDatabaseHas(t, "personal_access_tokens", map[string]any{
		"name":           "reader",
		"tokenable_id":   author.ID,
		"tokenable_type": "users",
	})
}

// The API authenticates with the token rather than the session.
func TestTheAPIAuthenticatesWithAToken(t *testing.T) {
	h := boot(t)
	author := h.author(t)
	token := h.token(t, author, "profile:read")

	h.api(t, token).Get("/api/blog/me").
		AssertOK().
		AssertJsonPath("data.user.email", author.Email).
		AssertJsonPath("data.token.name", "test")

	// The password hash never leaves the model, whatever serializes it.
	h.api(t, token).Get("/api/blog/me").AssertJsonMissing("data.user.password")
}

// No token, no API.
func TestTheAPIRefusesAnUnauthenticatedCaller(t *testing.T) {
	h := boot(t)

	h.tc.Get("/api/blog/posts").AssertUnauthorized()
	h.api(t, "not-a-real-token").Get("/api/blog/posts").AssertUnauthorized()
}

// A token carries abilities, and a route may require one.
func TestAnAbilityTheTokenDoesNotHaveIsForbidden(t *testing.T) {
	h := boot(t)
	author := h.author(t)
	readOnly := h.token(t, author, "profile:read")

	h.api(t, readOnly).Post("/api/blog/posts", map[string]any{
		"title": "Written by a token",
		"body":  "A body long enough to satisfy the rules of the form.",
	}).AssertForbidden()

	dbtest.AssertDatabaseCount(t, "posts", 0)

	// With the ability, the same request is accepted.
	writer := h.token(t, author, "profile:read", "posts:write")
	h.api(t, writer).Post("/api/blog/posts", map[string]any{
		"title": "Written by a token",
		"body":  "A body long enough to satisfy the rules of the form.",
	}).AssertCreated().AssertJsonPath("data.slug", "written-by-a-token")

	dbtest.AssertDatabaseHas(t, "posts", map[string]any{
		"slug":      "written-by-a-token",
		"author_id": author.ID,
	})
}

// The API renders through ToMap, so Hidden and Appends are honoured.
func TestTheAPIHidesAndAppendsFields(t *testing.T) {
	h := boot(t)
	author := h.author(t)
	post := h.published(t, author)

	h.api(t, h.token(t, author, "profile:read")).
		Get("/api/blog/posts/"+post.Slug).
		AssertOK().
		AssertJsonPath("data.slug", post.Slug).
		AssertJsonPath("data.published", true).
		AssertJsonMissing("data.deleted_at")
}

// The index is paginated, and lists published posts only.
func TestTheAPIIndexIsPaginated(t *testing.T) {
	h := boot(t)
	author := h.author(t)
	for i := 0; i < 3; i++ {
		h.published(t, author)
	}
	h.post(t, author)

	h.api(t, h.token(t, author, "profile:read")).
		Get("/api/blog/posts").
		AssertOK().
		AssertJsonCount("data", 3).
		AssertJsonPath("meta.total", 3).
		AssertJsonPath("meta.current_page", 1)
}

// A JSON client gets the 422 payload, not the browser's redirect.
func TestTheAPIReturnsValidationErrorsAsJSON(t *testing.T) {
	h := boot(t)
	author := h.author(t)

	h.api(t, h.token(t, author, "posts:write")).
		Post("/api/blog/posts", map[string]any{"title": "No", "body": "short"}).
		AssertValidationError("title").
		AssertValidationError("body")
}

// A revoked token stops working immediately.
func TestARevokedTokenIsRefused(t *testing.T) {
	h := boot(t)
	author := h.author(t)
	token := h.token(t, author, "profile:read")

	h.api(t, token).Get("/api/blog/me").AssertOK()

	found, err := h.tokens.Find(token)
	require.NoError(t, err)
	require.NoError(t, h.tokens.Revoke(found.ID))

	h.api(t, token).Get("/api/blog/me").AssertUnauthorized()
}

// The API is exempt from the CSRF check - it authenticates with a token,
// which a cross-site form cannot supply.
func TestTheAPIDoesNotRequireACSRFToken(t *testing.T) {
	h := boot(t)
	author := h.author(t)

	var post models.Post
	_ = post

	h.api(t, h.token(t, author, "posts:write")).
		Post("/api/blog/posts", map[string]any{
			"title": "No CSRF token in sight",
			"body":  "A body long enough to satisfy the rules of the form.",
		}).
		AssertCreated()
}

// A draft is hidden from the API too - the same policy decides.
func TestTheAPIHidesAnotherAuthorsDraft(t *testing.T) {
	h := boot(t)
	draft := h.post(t, h.author(t))

	stranger := h.author(t)
	h.api(t, h.token(t, stranger, "profile:read")).
		Get("/api/blog/posts/" + draft.Slug).
		AssertForbidden()

	assert.NotEqual(t, "", draft.Slug)
}
