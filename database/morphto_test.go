package database_test

import (
	"testing"

	"github.com/genesysflow/go-genesys/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

// Note is a comment that knows what it is attached to: the inverse of
// morphMany. The related type varies per row, so the field is an
// interface and the type column decides what fills it.
type Note struct {
	database.Model
	Body            string `db:"body"`
	CommentableType string `db:"commentable_type"`
	CommentableID   int64  `db:"commentable_id"`
	Commentable     any    `db:"-" rel:"morphTo,as:commentable"`
}

func setupMorphTo(t *testing.T) {
	t.Helper()
	setupRound4(t)

	manager := database.Default()
	_, err := manager.Statement(`CREATE TABLE notes (id INTEGER PRIMARY KEY AUTOINCREMENT, body TEXT NOT NULL,
		commentable_type TEXT NOT NULL, commentable_id INTEGER NOT NULL, created_at TIMESTAMP, updated_at TIMESTAMP)`)
	require.NoError(t, err)

	database.RegisterMorph[MorphPost]()
	database.RegisterMorph[MorphVideo]()
	t.Cleanup(database.ClearMorphs)
}

func TestMorphToLoadsEachParentType(t *testing.T) {
	setupMorphTo(t)

	post := &MorphPost{Title: "Hello"}
	require.NoError(t, database.Create(post))
	video := &MorphVideo{Title: "Clip"}
	require.NoError(t, database.Create(video))

	require.NoError(t, database.Create(&Note{Body: "on post", CommentableType: "morph_posts", CommentableID: post.ID}))
	require.NoError(t, database.Create(&Note{Body: "on video", CommentableType: "morph_videos", CommentableID: video.ID}))

	notes, err := database.Query[Note]().With("Commentable").OrderBy("id").Get()
	require.NoError(t, err)
	require.Len(t, notes, 2)

	loadedPost, ok := notes[0].Commentable.(*MorphPost)
	require.True(t, ok, "the first note should carry its post")
	assert.Equal(t, "Hello", loadedPost.Title)

	loadedVideo, ok := notes[1].Commentable.(*MorphVideo)
	require.True(t, ok, "the second note should carry its video")
	assert.Equal(t, "Clip", loadedVideo.Title)
}

// Every parent of one type is fetched in a single query, not one per row.
func TestMorphToBatchesByType(t *testing.T) {
	setupMorphTo(t)

	post := &MorphPost{Title: "Hello"}
	require.NoError(t, database.Create(post))

	for i := 0; i < 3; i++ {
		require.NoError(t, database.Create(&Note{Body: "note", CommentableType: "morph_posts", CommentableID: post.ID}))
	}

	var queries int
	database.Default().Listen(func(event database.QueryEvent) { queries++ })

	notes, err := database.Query[Note]().With("Commentable").Get()
	require.NoError(t, err)
	require.Len(t, notes, 3)

	// One query for the notes, one for the posts.
	assert.LessOrEqual(t, queries, 2, "morphTo should batch its parents per type")
}

// A type nobody registered cannot be turned into a model; the row is
// left unloaded rather than guessed at.
func TestMorphToUnknownTypeIsSkipped(t *testing.T) {
	setupMorphTo(t)

	require.NoError(t, database.Create(&Note{Body: "orphan", CommentableType: "nope", CommentableID: 1}))

	notes, err := database.Query[Note]().With("Commentable").Get()
	require.NoError(t, err)
	require.Len(t, notes, 1)
	assert.Nil(t, notes[0].Commentable)
}

// A parent row that has been deleted leaves the relation empty rather
// than failing the whole load.
func TestMorphToMissingParent(t *testing.T) {
	setupMorphTo(t)

	require.NoError(t, database.Create(&Note{Body: "dangling", CommentableType: "morph_posts", CommentableID: 999}))

	notes, err := database.Query[Note]().With("Commentable").Get()
	require.NoError(t, err)
	require.Len(t, notes, 1)
	assert.Nil(t, notes[0].Commentable)
}

// A morph alias lets the stored type string differ from the table name,
// so a table can be renamed without rewriting every stored row.
func TestRegisterMorphAlias(t *testing.T) {
	setupMorphTo(t)
	database.RegisterMorphAs[MorphPost]("post")

	post := &MorphPost{Title: "Aliased"}
	require.NoError(t, database.Create(post))
	require.NoError(t, database.Create(&Note{Body: "aliased", CommentableType: "post", CommentableID: post.ID}))

	notes, err := database.Query[Note]().With("Commentable").Get()
	require.NoError(t, err)
	require.Len(t, notes, 1)

	loaded, ok := notes[0].Commentable.(*MorphPost)
	require.True(t, ok)
	assert.Equal(t, "Aliased", loaded.Title)
}
