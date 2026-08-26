package tests

import (
	"testing"

	"github.com/genesysflow/go-genesys/database"
	"github.com/genesysflow/go-genesys/example/app/models"
	"github.com/genesysflow/go-genesys/testutil/dbtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A valid draft is written and the author lands on it.
func TestStoreCreatesADraft(t *testing.T) {
	h := boot(t)
	author := h.signIn(t, h.author(t))

	h.submit(t, "/drafts/new", "/posts", map[string]string{
		"title": "On the Analytical Engine",
		"body":  "A note about the engine, long enough to satisfy the rules.",
	}).AssertRedirect("/posts/on-the-analytical-engine")

	dbtest.AssertDatabaseHas(t, "posts", map[string]any{
		"slug":      "on-the-analytical-engine",
		"author_id": author.ID,
	})

	// A new post is a draft: nothing publishes it but publishing.
	post, err := database.Query[models.Post]().Where("slug", "on-the-analytical-engine").First()
	require.NoError(t, err)
	assert.Nil(t, post.PublishedAt)
}

// PrepareForValidation derives the slug from the title, so the field is
// optional to a human and required to the rules.
func TestStoreDerivesTheSlugFromTheTitle(t *testing.T) {
	h := boot(t)
	h.signIn(t, h.author(t))

	h.submit(t, "/drafts/new", "/posts", map[string]string{
		"title": "  Ada   &   the   Engine  ",
		"body":  "A note about the engine, long enough to satisfy the rules.",
	}).AssertRedirect()

	dbtest.AssertDatabaseHas(t, "posts", map[string]any{
		"title": "Ada & the Engine",
		"slug":  "ada-the-engine",
	})
}

// A validation failure goes back to the form, with the messages and the
// input the user already typed.
func TestStoreSendsAValidationFailureBackToTheForm(t *testing.T) {
	h := boot(t)
	h.signIn(t, h.author(t))

	h.submit(t, "/drafts/new", "/posts", map[string]string{
		"title": "Ok",
		"body":  "Too short.",
	}).AssertRedirect(testOrigin + "/drafts/new")

	dbtest.AssertDatabaseCount(t, "posts", 0)

	body := h.visit(t, "/drafts/new").AssertOK().BodyString()

	// The custom message from Messages(), not the default wording.
	assert.Contains(t, body, "Write at least a couple of sentences.")

	// The old input is back in the form.
	assert.Contains(t, body, `value="Ok"`)
}

// AfterValidation runs once the per-field rules pass, for the check no
// single field can make.
func TestStoreRejectsABodyThatOnlyRepeatsTheTitle(t *testing.T) {
	h := boot(t)
	h.signIn(t, h.author(t))

	repeated := "A title that is also the whole body here"
	h.submit(t, "/drafts/new", "/posts", map[string]string{
		"title": repeated,
		"body":  repeated,
	}).AssertRedirect(testOrigin + "/drafts/new")

	body := h.visit(t, "/drafts/new").AssertOK().BodyString()
	assert.Contains(t, body, "The post body must say more than its title.")
}

// The unique rule reaches the database, so a taken slug is refused.
func TestStoreRefusesATakenSlug(t *testing.T) {
	h := boot(t)
	author := h.signIn(t, h.author(t))
	existing := h.post(t, author)

	h.submit(t, "/drafts/new", "/posts", map[string]string{
		"title": "A different title entirely",
		"slug":  existing.Slug,
		"body":  "A note about the engine, long enough to satisfy the rules.",
	}).AssertRedirect(testOrigin + "/drafts/new")

	dbtest.AssertDatabaseCount(t, "posts", 1)
}

// Editing keeps the post's own slug: the uniqueness check has to ignore
// the row being edited, or every save collides with itself.
func TestUpdateKeepsItsOwnSlug(t *testing.T) {
	h := boot(t)
	author := h.signIn(t, h.author(t))
	post := h.post(t, author)

	response := h.editForm(t, post, map[string]string{
		"title": "A revised title",
		"slug":  post.Slug,
		"body":  "The revised body, still long enough to satisfy the rules.",
	})
	response.AssertRedirect("/posts/" + post.Slug)

	dbtest.AssertDatabaseHas(t, "posts", map[string]any{
		"id":    post.ID,
		"title": "A revised title",
	})
}

// Changing the slug to one another post holds is still refused.
func TestUpdateRefusesAnotherPostsSlug(t *testing.T) {
	h := boot(t)
	author := h.signIn(t, h.author(t))
	post := h.post(t, author)
	other := h.post(t, author)

	h.editForm(t, post, map[string]string{
		"title": "A revised title",
		"slug":  other.Slug,
		"body":  "The revised body, still long enough to satisfy the rules.",
	}).AssertRedirect(testOrigin + "/posts/" + post.Slug + "/edit")

	fresh, err := database.Find[models.Post](post.ID)
	require.NoError(t, err)
	assert.Equal(t, post.Slug, fresh.Slug, "the slug should not have changed")
}

// Deleting is a soft delete: the row stays, marked.
func TestDestroySoftDeletesThePost(t *testing.T) {
	h := boot(t)
	author := h.signIn(t, h.author(t))
	post := h.post(t, author)

	h.visit(t, "/posts/"+post.Slug).AssertOK()
	h.tc.Do(h.formRequest(t, "DELETE", "/posts/"+post.Slug, nil)).
		AssertRedirect("/posts")

	dbtest.AssertSoftDeleted(t, "posts", map[string]any{"id": post.ID})

	// And it is gone from the site.
	h.visit(t, "/posts/"+post.Slug).AssertNotFound()
}

// The index lists published posts only.
func TestIndexShowsPublishedPostsOnly(t *testing.T) {
	h := boot(t)
	author := h.author(t)
	live := h.published(t, author)
	draft := h.post(t, author)

	body := h.visit(t, "/posts").AssertOK().BodyString()
	assert.Contains(t, body, live.Title)
	assert.NotContains(t, body, draft.Title)
}

// A draft is nobody's business but its author's and the editors'.
func TestADraftIsHiddenFromOtherReaders(t *testing.T) {
	h := boot(t)
	author := h.author(t)
	draft := h.post(t, author)

	// A stranger.
	h.signIn(t, h.author(t))
	h.visit(t, "/posts/"+draft.Slug).AssertForbidden()

	// Its author.
	h.tc.FlushCookies()
	h.signIn(t, author)
	h.visit(t, "/posts/"+draft.Slug).AssertOK()
}

// The Edit form is an HTML form, so it reaches PUT the only way a
// browser can: as a POST declaring the verb. This is the path a reader
// takes, and until method override existed it answered 405.
func TestEditingThroughTheBrowsersForm(t *testing.T) {
	h := boot(t)
	author := h.author(t)
	post := h.post(t, author)

	h.signIn(t, author)
	h.visit(t, "/posts/"+post.Slug+"/edit").AssertOK()

	h.browserForm(t, "PUT", "/posts/"+post.Slug, map[string]string{
		"title": "Retitled from the browser",
		"slug":  post.Slug,
		"body":  "A body long enough to satisfy the rules of the form.",
	}).AssertRedirect()

	dbtest.AssertDatabaseHas(t, "posts", map[string]any{
		"id":    post.ID,
		"title": "Retitled from the browser",
	})
}

// And so does the Delete button.
func TestDeletingThroughTheBrowsersForm(t *testing.T) {
	h := boot(t)
	author := h.author(t)
	post := h.post(t, author)

	h.signIn(t, author)
	h.visit(t, "/posts/"+post.Slug).AssertOK()

	h.browserForm(t, "DELETE", "/posts/"+post.Slug, nil).
		AssertRedirect("/posts")

	dbtest.AssertSoftDeleted(t, "posts", map[string]any{"id": post.ID})
	h.visit(t, "/posts/"+post.Slug).AssertNotFound()
}

// A draft has somewhere to be found again: its author's own list.
func TestDraftsListsTheAuthorsOwnUnpublishedPosts(t *testing.T) {
	h := boot(t)
	author := h.author(t)
	mine := h.post(t, author)
	live := h.published(t, author)
	theirs := h.post(t, h.author(t))

	h.signIn(t, author)

	body := h.visit(t, "/drafts").AssertOK().BodyString()
	assert.Contains(t, body, mine.Title)
	assert.NotContains(t, body, live.Title)
	assert.NotContains(t, body, theirs.Title)
}

// An editor is looking at the whole desk, not one author's corner of it.
func TestDraftsShowsEveryAuthorsWorkToAnEditor(t *testing.T) {
	h := boot(t)
	theirs := h.post(t, h.author(t))

	h.signIn(t, h.editor(t))

	assert.Contains(t, h.visit(t, "/drafts").AssertOK().BodyString(), theirs.Title)
}

// Unpublished work is not for guests, whoever wrote it.
func TestDraftsSendsAGuestToSignIn(t *testing.T) {
	h := boot(t)
	h.post(t, h.author(t))

	h.tc.Get("/drafts").AssertRedirect("/login")
}

// Comments are written against the post they were posted from.
func TestCommentingOnAPost(t *testing.T) {
	h := boot(t)
	author := h.author(t)
	post := h.published(t, author)

	reader := h.signIn(t, h.author(t))

	h.visit(t, "/posts/"+post.Slug).AssertOK()
	h.form(t, "/posts/"+post.Slug+"/comments", map[string]string{
		"body": "A thoughtful reply.",
	}).AssertRedirect(testOrigin + "/posts/" + post.Slug)

	dbtest.AssertDatabaseHas(t, "comments", map[string]any{
		"post_id":   post.ID,
		"author_id": reader.ID,
		"body":      "A thoughtful reply.",
	})

	assert.Contains(t, h.visit(t, "/posts/"+post.Slug).BodyString(), "A thoughtful reply.")
}

// An empty comment goes back to the post, and writes nothing.
func TestAnEmptyCommentIsRefused(t *testing.T) {
	h := boot(t)
	post := h.published(t, h.author(t))
	h.signIn(t, h.author(t))

	h.visit(t, "/posts/"+post.Slug).AssertOK()
	h.form(t, "/posts/"+post.Slug+"/comments", map[string]string{"body": ""}).
		AssertRedirect(testOrigin + "/posts/" + post.Slug)

	dbtest.AssertDatabaseCount(t, "comments", 0)
}
