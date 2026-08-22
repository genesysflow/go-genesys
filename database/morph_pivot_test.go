package database_test

import (
	"testing"

	"github.com/genesysflow/go-genesys/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

// A morphToMany relation is written through the same pivot helpers as a
// belongsToMany, and its type column has to be filled in - otherwise the
// link is invisible to every read, which filters on it.
func TestAttachMorphToMany(t *testing.T) {
	setupRound4(t)

	post := &MorphPost{Title: "Hello"}
	require.NoError(t, database.Create(post))

	goTag := &MorphTag{Name: "go"}
	webTag := &MorphTag{Name: "web"}
	require.NoError(t, database.Create(goTag))
	require.NoError(t, database.Create(webTag))

	require.NoError(t, database.Attach(post, "Tags", goTag.ID, webTag.ID))

	loaded, err := database.Query[MorphPost]().With("Tags").Where("id", post.ID).First()
	require.NoError(t, err)
	require.Len(t, loaded.Tags, 2, "attached tags should load back")
}

// Attaching the same tag twice leaves one link.
func TestAttachMorphToManyIsIdempotent(t *testing.T) {
	setupRound4(t)

	post := &MorphPost{Title: "Hello"}
	require.NoError(t, database.Create(post))
	tag := &MorphTag{Name: "go"}
	require.NoError(t, database.Create(tag))

	require.NoError(t, database.Attach(post, "Tags", tag.ID))
	require.NoError(t, database.Attach(post, "Tags", tag.ID))

	loaded, err := database.Query[MorphPost]().With("Tags").Where("id", post.ID).First()
	require.NoError(t, err)
	assert.Len(t, loaded.Tags, 1)
}

// A link belongs to one parent type: detaching a post's tag must not
// touch a video's, even at the same id.
func TestDetachMorphToManyIsScopedToItsType(t *testing.T) {
	setupRound4(t)

	post := &MorphPost{Title: "Hello"}
	require.NoError(t, database.Create(post))
	tag := &MorphTag{Name: "go"}
	require.NoError(t, database.Create(tag))

	require.NoError(t, database.Attach(post, "Tags", tag.ID))

	// A link for a different type at the same id.
	manager := database.Default()
	_, err := manager.Statement(
		`INSERT INTO taggables (tag_id, taggable_id, taggable_type) VALUES (?, ?, ?)`,
		tag.ID, post.ID, "morph_videos",
	)
	require.NoError(t, err)

	removed, err := database.Detach(post, "Tags", tag.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(1), removed, "only the post's link should be removed")

	var remaining int
	row := manager.Connection().QueryRow(`SELECT COUNT(*) FROM taggables WHERE taggable_type = 'morph_videos'`)
	require.NoError(t, row.Scan(&remaining))
	assert.Equal(t, 1, remaining, "another type's link must survive")
}

// Sync replaces the set, again scoped to this parent's type.
func TestSyncMorphToMany(t *testing.T) {
	setupRound4(t)

	post := &MorphPost{Title: "Hello"}
	require.NoError(t, database.Create(post))

	goTag := &MorphTag{Name: "go"}
	webTag := &MorphTag{Name: "web"}
	require.NoError(t, database.Create(goTag))
	require.NoError(t, database.Create(webTag))

	require.NoError(t, database.Attach(post, "Tags", goTag.ID))
	require.NoError(t, database.Sync(post, "Tags", webTag.ID))

	loaded, err := database.Query[MorphPost]().With("Tags").Where("id", post.ID).First()
	require.NoError(t, err)
	require.Len(t, loaded.Tags, 1)
	assert.Equal(t, "web", loaded.Tags[0].Name)
}
