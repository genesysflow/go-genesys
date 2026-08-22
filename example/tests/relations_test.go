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

// Tags are attached polymorphically, so the same tag can label a post
// today and something else tomorrow without a second pivot table.
func TestTaggingAPost(t *testing.T) {
	h := boot(t)
	post := h.post(t, h.author(t))

	golang, err := factories.Tags.CreateOne(func(tag *models.Tag) {
		tag.Name, tag.Slug = "Go", "go"
	})
	require.NoError(t, err)
	web, err := factories.Tags.CreateOne(func(tag *models.Tag) {
		tag.Name, tag.Slug = "Web", "web"
	})
	require.NoError(t, err)

	require.NoError(t, database.Attach(post, "Tags", golang.ID, web.ID))

	dbtest.AssertDatabaseCount(t, "taggables", 2)
	dbtest.AssertDatabaseHas(t, "taggables", map[string]any{
		"tag_id":        golang.ID,
		"taggable_id":   post.ID,
		"taggable_type": "posts",
	})

	// Eager loading brings them back.
	loaded, err := database.Query[models.Post]().With("Tags").Where("id", post.ID).First()
	require.NoError(t, err)
	require.Len(t, loaded.Tags, 2)
}

// Detaching removes only this post's link, leaving the tag itself and
// anyone else's use of it alone.
func TestDetachingATag(t *testing.T) {
	h := boot(t)
	author := h.author(t)
	post := h.post(t, author)
	other := h.post(t, author)

	tag, err := factories.Tags.CreateOne()
	require.NoError(t, err)

	require.NoError(t, database.Attach(post, "Tags", tag.ID))
	require.NoError(t, database.Attach(other, "Tags", tag.ID))
	detached, err := database.Detach(post, "Tags", tag.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(1), detached)

	dbtest.AssertDatabaseCount(t, "taggables", 1)
	dbtest.AssertDatabaseHas(t, "tags", map[string]any{"id": tag.ID})
	dbtest.AssertDatabaseHas(t, "taggables", map[string]any{"taggable_id": other.ID})
}

// Sync replaces the set: what is not in the list goes.
func TestSyncingTags(t *testing.T) {
	h := boot(t)
	post := h.post(t, h.author(t))

	first, err := factories.Tags.CreateOne()
	require.NoError(t, err)
	second, err := factories.Tags.CreateOne()
	require.NoError(t, err)
	third, err := factories.Tags.CreateOne()
	require.NoError(t, err)

	require.NoError(t, database.Attach(post, "Tags", first.ID, second.ID))
	require.NoError(t, database.Sync(post, "Tags", second.ID, third.ID))

	loaded, err := database.Query[models.Post]().With("Tags").Where("id", post.ID).First()
	require.NoError(t, err)

	ids := make([]int64, 0, len(loaded.Tags))
	for _, tag := range loaded.Tags {
		ids = append(ids, tag.ID)
	}
	assert.ElementsMatch(t, []int64{second.ID, third.ID}, ids)
}

// An attachment names its owner's type, so the inverse relation resolves
// a post or a comment from the same table.
func TestAnAttachmentResolvesItsOwner(t *testing.T) {
	h := boot(t)
	author := h.author(t)
	post := h.post(t, author)

	comment := &models.Comment{PostID: post.ID, AuthorID: author.ID, Body: "A reply."}
	require.NoError(t, database.Create(comment))

	onPost := &models.Attachment{
		AttachableType: "posts", AttachableID: post.ID,
		Disk: "local", Path: "diagram.png", Size: 2048,
	}
	onComment := &models.Attachment{
		AttachableType: "comments", AttachableID: comment.ID,
		Disk: "local", Path: "reply.png", Size: 512,
	}
	require.NoError(t, database.Create(onPost))
	require.NoError(t, database.Create(onComment))

	loaded, err := database.Query[models.Attachment]().With("Attachable").OrderBy("id").Get()
	require.NoError(t, err)
	require.Len(t, loaded, 2)

	owningPost, ok := loaded[0].Attachable.(*models.Post)
	require.True(t, ok, "the first attachment should belong to a post, got %T", loaded[0].Attachable)
	assert.Equal(t, post.ID, owningPost.ID)

	owningComment, ok := loaded[1].Attachable.(*models.Comment)
	require.True(t, ok, "the second attachment should belong to a comment, got %T", loaded[1].Attachable)
	assert.Equal(t, comment.ID, owningComment.ID)
}

// A post's own attachments come back through the morphMany side.
func TestAPostsAttachments(t *testing.T) {
	h := boot(t)
	post := h.post(t, h.author(t))

	require.NoError(t, database.Create(&models.Attachment{
		AttachableType: "posts", AttachableID: post.ID,
		Disk: "local", Path: "diagram.png", Size: 2048,
	}))

	loaded, err := database.Query[models.Post]().With("Attachments").Where("id", post.ID).First()
	require.NoError(t, err)
	require.Len(t, loaded.Attachments, 1)
	assert.Equal(t, "diagram.png", loaded.Attachments[0].Path)
}

// WithCount counts the related rows without loading them.
func TestCountingComments(t *testing.T) {
	h := boot(t)
	author := h.author(t)
	post := h.published(t, author)

	for i := 0; i < 3; i++ {
		require.NoError(t, database.Create(&models.Comment{
			PostID: post.ID, AuthorID: author.ID, Body: "A reply.",
		}))
	}

	loaded, err := database.Query[models.Post]().WithCount("Comments").Where("id", post.ID).First()
	require.NoError(t, err)
	assert.Equal(t, int64(3), loaded.CommentsCount)
}

// Hidden and Appends travel with the model, whatever serializes it.
func TestSerializingAPost(t *testing.T) {
	h := boot(t)
	post := h.published(t, h.author(t))

	rendered := database.ToMap(post)

	assert.Equal(t, post.Slug, rendered["slug"])
	assert.Equal(t, true, rendered["published"])
	assert.NotContains(t, rendered, "deleted_at")
	assert.Contains(t, rendered, "excerpt")
}

// Tags render through a view component, so the markup for one lives in
// one place.
func TestTagsRenderThroughTheComponent(t *testing.T) {
	h := boot(t)
	post := h.published(t, h.author(t))

	tag, err := factories.Tags.CreateOne(func(tag *models.Tag) {
		tag.Name, tag.Slug = "Go", "go"
	})
	require.NoError(t, err)
	require.NoError(t, database.Attach(post, "Tags", tag.ID))

	body := h.visit(t, "/posts").AssertOK().BodyString()
	assert.Contains(t, body, `<span class="tag tag-go">Go</span>`)
}
