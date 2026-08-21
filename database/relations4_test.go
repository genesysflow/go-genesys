package database_test

import (
	"testing"

	"github.com/genesysflow/go-genesys/database"
	"github.com/genesysflow/go-genesys/query"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

// --- polymorphic fixtures ---

type MorphPost struct {
	database.Model
	Title         string          `db:"title"`
	Comments      []*MorphComment `rel:"morphMany,as:commentable"`
	Image         *MorphImage     `rel:"morphOne,as:imageable"`
	Tags          []MorphTag      `rel:"morphToMany,as:taggable,prk:tag_id"`
	CommentsCount int64           `db:"-"`
	TagsCount     int64           `db:"-"`
}

type MorphVideo struct {
	database.Model
	Title    string          `db:"title"`
	Comments []*MorphComment `rel:"morphMany,as:commentable"`
}

type MorphComment struct {
	database.Model
	Body            string `db:"body"`
	CommentableType string `db:"commentable_type"`
	CommentableID   int64  `db:"commentable_id"`
}

type MorphImage struct {
	database.Model
	URL           string `db:"url"`
	ImageableType string `db:"imageable_type"`
	ImageableID   int64  `db:"imageable_id"`
}

type MorphTag struct {
	database.Model
	Name string `db:"name"`
}

// --- through fixtures: nations -> writers -> essays ---

type Nation struct {
	database.Model
	Name        string   `db:"name"`
	Essays      []*Essay `rel:"hasManyThrough,through:writers"`
	FirstEssay  *Essay   `rel:"hasOneThrough,through:writers"`
	EssaysCount int64    `db:"-"`
}

type Writer struct {
	database.Model
	NationID int64  `db:"nation_id"`
	Name     string `db:"name"`
}

type Essay struct {
	database.Model
	WriterID int64  `db:"writer_id"`
	Title    string `db:"title"`
}

func setupRound4(t *testing.T) {
	t.Helper()
	setupORM(t)
	manager := database.Default()
	for _, stmt := range []string{
		`CREATE TABLE morph_posts (id INTEGER PRIMARY KEY AUTOINCREMENT, title TEXT NOT NULL, created_at TIMESTAMP, updated_at TIMESTAMP)`,
		`CREATE TABLE morph_videos (id INTEGER PRIMARY KEY AUTOINCREMENT, title TEXT NOT NULL, created_at TIMESTAMP, updated_at TIMESTAMP)`,
		`CREATE TABLE morph_comments (id INTEGER PRIMARY KEY AUTOINCREMENT, body TEXT NOT NULL,
			commentable_type TEXT NOT NULL, commentable_id INTEGER NOT NULL, created_at TIMESTAMP, updated_at TIMESTAMP)`,
		`CREATE TABLE morph_images (id INTEGER PRIMARY KEY AUTOINCREMENT, url TEXT NOT NULL,
			imageable_type TEXT NOT NULL, imageable_id INTEGER NOT NULL, created_at TIMESTAMP, updated_at TIMESTAMP)`,
		`CREATE TABLE morph_tags (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL, created_at TIMESTAMP, updated_at TIMESTAMP)`,
		`CREATE TABLE taggables (tag_id INTEGER NOT NULL, taggable_id INTEGER NOT NULL, taggable_type TEXT NOT NULL)`,
		`CREATE TABLE nations (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL, created_at TIMESTAMP, updated_at TIMESTAMP)`,
		`CREATE TABLE writers (id INTEGER PRIMARY KEY AUTOINCREMENT, nation_id INTEGER NOT NULL, name TEXT NOT NULL, created_at TIMESTAMP, updated_at TIMESTAMP)`,
		`CREATE TABLE essays (id INTEGER PRIMARY KEY AUTOINCREMENT, writer_id INTEGER NOT NULL, title TEXT NOT NULL, created_at TIMESTAMP, updated_at TIMESTAMP)`,
	} {
		_, err := manager.Statement(stmt)
		require.NoError(t, err)
	}
}

func TestMorphManyAndMorphOne(t *testing.T) {
	setupRound4(t)

	post := &MorphPost{Title: "Hello"}
	require.NoError(t, database.Create(post))
	video := &MorphVideo{Title: "Clip"}
	require.NoError(t, database.Create(video))

	// Comments on the post, the video, and an image on the post. The
	// morph type column carries the parent's table name.
	require.NoError(t, database.Create(&MorphComment{Body: "on post", CommentableType: "morph_posts", CommentableID: post.ID}))
	require.NoError(t, database.Create(&MorphComment{Body: "also on post", CommentableType: "morph_posts", CommentableID: post.ID}))
	require.NoError(t, database.Create(&MorphComment{Body: "on video", CommentableType: "morph_videos", CommentableID: post.ID}))
	require.NoError(t, database.Create(&MorphImage{URL: "/p.png", ImageableType: "morph_posts", ImageableID: post.ID}))

	posts, err := database.Query[MorphPost]().With("Comments", "Image").Get()
	require.NoError(t, err)
	require.Len(t, posts, 1)
	require.Len(t, posts[0].Comments, 2, "only comments whose morph type matches load")
	assert.Equal(t, "on post", posts[0].Comments[0].Body)
	require.NotNil(t, posts[0].Image)
	assert.Equal(t, "/p.png", posts[0].Image.URL)

	// The video sees only its own comment, even though it shares the id.
	videos, err := database.Query[MorphVideo]().With("Comments").Get()
	require.NoError(t, err)
	require.Len(t, videos, 1)
	require.Len(t, videos[0].Comments, 1)
	assert.Equal(t, "on video", videos[0].Comments[0].Body)
}

func TestMorphToMany(t *testing.T) {
	setupRound4(t)
	manager := database.Default()

	post := &MorphPost{Title: "Tagged"}
	require.NoError(t, database.Create(post))
	bare := &MorphPost{Title: "Untagged"}
	require.NoError(t, database.Create(bare))
	video := &MorphVideo{Title: "Clip"}
	require.NoError(t, database.Create(video))

	goTag := &MorphTag{Name: "go"}
	require.NoError(t, database.Create(goTag))
	webTag := &MorphTag{Name: "web"}
	require.NoError(t, database.Create(webTag))

	for _, row := range []struct {
		tag, target int64
		kind        string
	}{
		{goTag.ID, post.ID, "morph_posts"},
		{webTag.ID, post.ID, "morph_posts"},
		{goTag.ID, video.ID, "morph_videos"},
	} {
		_, err := manager.Statement(
			`INSERT INTO taggables (tag_id, taggable_id, taggable_type) VALUES (?, ?, ?)`,
			row.tag, row.target, row.kind)
		require.NoError(t, err)
	}

	posts, err := database.Query[MorphPost]().With("Tags").OrderBy("id").Get()
	require.NoError(t, err)
	require.Len(t, posts, 2)
	assert.Len(t, posts[0].Tags, 2, "the tagged post has both tags")
	assert.Empty(t, posts[1].Tags)

	// Has() honours the morph type on the pivot.
	tagged, err := database.Query[MorphPost]().Has("Tags").Get()
	require.NoError(t, err)
	require.Len(t, tagged, 1)
	assert.Equal(t, "Tagged", tagged[0].Title)
}

func TestMorphHasAndWhereHas(t *testing.T) {
	setupRound4(t)

	commented := &MorphPost{Title: "Discussed"}
	require.NoError(t, database.Create(commented))
	quiet := &MorphPost{Title: "Quiet"}
	require.NoError(t, database.Create(quiet))
	video := &MorphVideo{Title: "Clip"}
	require.NoError(t, database.Create(video))

	require.NoError(t, database.Create(&MorphComment{Body: "nice", CommentableType: "morph_posts", CommentableID: commented.ID}))
	// A video comment sharing the quiet post's id must NOT count for it.
	require.NoError(t, database.Create(&MorphComment{Body: "video!", CommentableType: "morph_videos", CommentableID: quiet.ID}))

	withComments, err := database.Query[MorphPost]().Has("Comments").Get()
	require.NoError(t, err)
	require.Len(t, withComments, 1)
	assert.Equal(t, "Discussed", withComments[0].Title)

	matched, err := database.Query[MorphPost]().
		WhereHas("Comments", func(c *query.Builder) { c.Where("body", "nice") }).Get()
	require.NoError(t, err)
	require.Len(t, matched, 1)

	none, err := database.Query[MorphPost]().DoesntHave("Comments").Get()
	require.NoError(t, err)
	require.Len(t, none, 1)
	assert.Equal(t, "Quiet", none[0].Title)
}

func TestHasManyThroughAndHasOneThrough(t *testing.T) {
	setupRound4(t)

	fr := &Nation{Name: "France"}
	require.NoError(t, database.Create(fr))
	jp := &Nation{Name: "Japan"}
	require.NoError(t, database.Create(jp))

	hugo := &Writer{NationID: fr.ID, Name: "Hugo"}
	require.NoError(t, database.Create(hugo))
	zola := &Writer{NationID: fr.ID, Name: "Zola"}
	require.NoError(t, database.Create(zola))
	soseki := &Writer{NationID: jp.ID, Name: "Soseki"}
	require.NoError(t, database.Create(soseki))

	require.NoError(t, database.Create(&Essay{WriterID: hugo.ID, Title: "On Misery"}))
	require.NoError(t, database.Create(&Essay{WriterID: zola.ID, Title: "J'accuse"}))
	require.NoError(t, database.Create(&Essay{WriterID: soseki.ID, Title: "Kokoro Notes"}))

	nations, err := database.Query[Nation]().With("Essays", "FirstEssay").OrderBy("id").Get()
	require.NoError(t, err)
	require.Len(t, nations, 2)
	assert.Len(t, nations[0].Essays, 2, "France's essays arrive through its writers")
	assert.Len(t, nations[1].Essays, 1)
	require.NotNil(t, nations[0].FirstEssay)

	// Has() through the intermediate table.
	empty := &Nation{Name: "Atlantis"}
	require.NoError(t, database.Create(empty))
	withEssays, err := database.Query[Nation]().Has("Essays").OrderBy("id").Get()
	require.NoError(t, err)
	assert.Len(t, withEssays, 2)

	without, err := database.Query[Nation]().DoesntHave("Essays").Get()
	require.NoError(t, err)
	require.Len(t, without, 1)
	assert.Equal(t, "Atlantis", without[0].Name)
}

func TestWithCount(t *testing.T) {
	setupRound4(t)
	manager := database.Default()

	busy := &MorphPost{Title: "Busy"}
	require.NoError(t, database.Create(busy))
	calm := &MorphPost{Title: "Calm"}
	require.NoError(t, database.Create(calm))

	for i := 0; i < 3; i++ {
		require.NoError(t, database.Create(&MorphComment{Body: "c", CommentableType: "morph_posts", CommentableID: busy.ID}))
	}
	require.NoError(t, database.Create(&MorphComment{Body: "c", CommentableType: "morph_videos", CommentableID: busy.ID}))

	tag := &MorphTag{Name: "go"}
	require.NoError(t, database.Create(tag))
	_, err := manager.Statement(
		`INSERT INTO taggables (tag_id, taggable_id, taggable_type) VALUES (?, ?, 'morph_posts')`, tag.ID, busy.ID)
	require.NoError(t, err)

	posts, err := database.Query[MorphPost]().WithCount("Comments", "Tags").OrderBy("id").Get()
	require.NoError(t, err)
	require.Len(t, posts, 2)
	assert.EqualValues(t, 3, posts[0].CommentsCount, "the video comment must not count")
	assert.EqualValues(t, 1, posts[0].TagsCount)
	assert.EqualValues(t, 0, posts[1].CommentsCount)
	assert.EqualValues(t, 0, posts[1].TagsCount)

	// Through counts.
	fr := &Nation{Name: "France"}
	require.NoError(t, database.Create(fr))
	w := &Writer{NationID: fr.ID, Name: "Hugo"}
	require.NoError(t, database.Create(w))
	require.NoError(t, database.Create(&Essay{WriterID: w.ID, Title: "One"}))
	require.NoError(t, database.Create(&Essay{WriterID: w.ID, Title: "Two"}))

	nations, err := database.Query[Nation]().WithCount("Essays").Get()
	require.NoError(t, err)
	require.Len(t, nations, 1)
	assert.EqualValues(t, 2, nations[0].EssaysCount)
}
