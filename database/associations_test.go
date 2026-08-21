package database_test

import (
	"testing"

	"github.com/genesysflow/go-genesys/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

func tagNames(t *testing.T, article *Article) []string {
	t.Helper()
	require.NoError(t, database.Load(article, "Tags"))
	names := make([]string, len(article.Tags))
	for i, tag := range article.Tags {
		names[i] = tag.Name
	}
	return names
}

func TestAttachDetach(t *testing.T) {
	setupBlog(t)
	article := &Article{Title: "Pivot"}
	require.NoError(t, database.Create(article))
	golang := &Tag{Name: "go"}
	web := &Tag{Name: "web"}
	require.NoError(t, database.Create(golang))
	require.NoError(t, database.Create(web))

	// Attach accepts models and raw ids, and is idempotent.
	require.NoError(t, database.Attach(article, "Tags", golang, web.ID))
	require.NoError(t, database.Attach(article, "Tags", golang))
	assert.ElementsMatch(t, []string{"go", "web"}, tagNames(t, article))

	count, err := database.Default().Table("article_tag").Count()
	require.NoError(t, err)
	assert.EqualValues(t, 2, count, "no duplicate pivot rows")

	// Detach specific, then all.
	removed, err := database.Detach(article, "Tags", golang)
	require.NoError(t, err)
	assert.EqualValues(t, 1, removed)
	assert.Equal(t, []string{"web"}, tagNames(t, article))

	removed, err = database.Detach(article, "Tags")
	require.NoError(t, err)
	assert.EqualValues(t, 1, removed)
	assert.Empty(t, tagNames(t, article))
}

func TestSyncAndToggle(t *testing.T) {
	setupBlog(t)
	article := &Article{Title: "Sync"}
	require.NoError(t, database.Create(article))
	a := &Tag{Name: "a"}
	b := &Tag{Name: "b"}
	c := &Tag{Name: "c"}
	for _, tag := range []*Tag{a, b, c} {
		require.NoError(t, database.Create(tag))
	}

	require.NoError(t, database.Attach(article, "Tags", a, b))

	// Sync to {b, c}: a detached, c attached, b untouched.
	require.NoError(t, database.Sync(article, "Tags", b, c))
	assert.ElementsMatch(t, []string{"b", "c"}, tagNames(t, article))

	// Toggle {a, b}: a attached (was gone), b detached (was there).
	require.NoError(t, database.Toggle(article, "Tags", a, b))
	assert.ElementsMatch(t, []string{"a", "c"}, tagNames(t, article))

	// Sync to nothing clears the pivot.
	require.NoError(t, database.Sync(article, "Tags"))
	assert.Empty(t, tagNames(t, article))
}

func TestCreateFor(t *testing.T) {
	setupBlog(t)
	author := &Author{Name: "Writer"}
	require.NoError(t, database.Create(author))

	post := &Article{Title: "Through relation"}
	require.NoError(t, database.CreateFor(author, "Posts", post))

	assert.NotZero(t, post.ID)
	assert.Equal(t, author.ID, post.AuthorID, "foreign key filled from the parent")

	require.NoError(t, database.Load(author, "Posts"))
	require.Len(t, author.Posts, 1)
	assert.Equal(t, "Through relation", author.Posts[0].Title)

	// hasOne works the same way.
	profile := &Profile{Bio: "bio"}
	require.NoError(t, database.CreateFor(author, "Profile", profile))
	assert.Equal(t, author.ID, profile.AuthorID)
}

func TestAssociateDissociate(t *testing.T) {
	setupBlog(t)
	author := &Author{Name: "Owner"}
	require.NoError(t, database.Create(author))

	article := &Article{Title: "Orphan"}
	require.NoError(t, database.Associate(article, "Author", author))
	assert.Equal(t, author.ID, article.AuthorID)
	require.NotNil(t, article.Author)
	assert.Equal(t, "Owner", article.Author.Name)

	require.NoError(t, database.Create(article))
	fetched, err := database.Query[Article]().With("Author").Find(article.ID)
	require.NoError(t, err)
	assert.Equal(t, "Owner", fetched.Author.Name)

	require.NoError(t, database.Dissociate(article, "Author"))
	assert.Zero(t, article.AuthorID)
	assert.Nil(t, article.Author)
}

func TestAssociationErrors(t *testing.T) {
	setupBlog(t)
	article := &Article{Title: "x"}
	author := &Author{Name: "y"}

	// Unsaved parent cannot take pivot links.
	assert.ErrorContains(t, database.Attach(article, "Tags", 1), "unsaved model")

	require.NoError(t, database.Create(article))
	// Wrong relation kinds are rejected.
	assert.ErrorContains(t, database.Attach(article, "Author", 1), "not belongsToMany")
	assert.ErrorContains(t, database.CreateFor(article, "Tags", &Tag{}), "hasOne/hasMany")
	assert.ErrorContains(t, database.Associate(article, "Tags", author), "belongsTo")
	// Unknown relations are rejected.
	assert.ErrorContains(t, database.Attach(article, "Nope", 1), "no relation")
	// Type mismatches are rejected.
	require.NoError(t, database.Create(author))
	assert.ErrorContains(t, database.CreateFor(author, "Posts", &Tag{}), "holds Article, not Tag")
}
