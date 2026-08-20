package database_test

import (
	"testing"

	"github.com/genesysflow/go-genesys/database"
	"github.com/genesysflow/go-genesys/query"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

func namesOf(authors []Author) []string {
	names := make([]string, len(authors))
	for i, a := range authors {
		names[i] = a.Name
	}
	return names
}

func TestHasAndDoesntHave(t *testing.T) {
	setupBlog(t)
	seedBlog(t)
	// Alice has 2 posts + a profile; Bob has 1 post, no profile.
	carol := &Author{Name: "Carol"} // no posts at all
	require.NoError(t, database.Create(carol))

	withPosts, err := database.Query[Author]().Has("Posts").OrderBy("name").Get()
	require.NoError(t, err)
	assert.Equal(t, []string{"Alice", "Bob"}, namesOf(withPosts))

	withoutPosts, err := database.Query[Author]().DoesntHave("Posts").Get()
	require.NoError(t, err)
	assert.Equal(t, []string{"Carol"}, namesOf(withoutPosts))

	withProfile, err := database.Query[Author]().Has("Profile").Get()
	require.NoError(t, err)
	assert.Equal(t, []string{"Alice"}, namesOf(withProfile))
}

func TestWhereHasWithConstraints(t *testing.T) {
	setupBlog(t)
	seedBlog(t)

	// Only Alice has a post titled "First".
	authors, err := database.Query[Author]().
		WhereHas("Posts", func(posts *query.Builder) {
			posts.Where("title", "First")
		}).Get()
	require.NoError(t, err)
	assert.Equal(t, []string{"Alice"}, namesOf(authors))

	// Nobody has a post with an unknown title.
	authors, err = database.Query[Author]().
		WhereHas("Posts", func(posts *query.Builder) {
			posts.Where("title", "Nope")
		}).Get()
	require.NoError(t, err)
	assert.Empty(t, authors)

	// OrWhereHas widens the match.
	authors, err = database.Query[Author]().
		Where("name", "Nonexistent").
		OrWhereHas("Posts", func(posts *query.Builder) {
			posts.Where("title", "Bob's")
		}).Get()
	require.NoError(t, err)
	assert.Equal(t, []string{"Bob"}, namesOf(authors))
}

func TestHasThroughBelongsToMany(t *testing.T) {
	setupBlog(t)
	seedBlog(t)

	// Articles tagged at all: First (go,web) and Second (go); Bob's is untagged.
	tagged, err := database.Query[Article]().Has("Tags").OrderBy("id").Get()
	require.NoError(t, err)
	require.Len(t, tagged, 2)

	// Constrain the pivot-joined related table.
	webTagged, err := database.Query[Article]().
		WhereHas("Tags", func(tags *query.Builder) {
			tags.Where("tags.name", "web")
		}).Get()
	require.NoError(t, err)
	require.Len(t, webTagged, 1)
	assert.Equal(t, "First", webTagged[0].Title)

	// Inverse: tags attached to any article.
	usedTags, err := database.Query[Tag]().Has("Articles").OrderBy("name").Get()
	require.NoError(t, err)
	assert.Len(t, usedTags, 2)
}

func TestNestedWhereHas(t *testing.T) {
	setupBlog(t)
	seedBlog(t)

	// Authors who have a post tagged "web": only Alice (via "First").
	authors, err := database.Query[Author]().
		WhereHas("Posts.Tags", func(tags *query.Builder) {
			tags.Where("tags.name", "web")
		}).Get()
	require.NoError(t, err)
	assert.Equal(t, []string{"Alice"}, namesOf(authors))

	// Has with a nested path and no constraint.
	authors, err = database.Query[Author]().Has("Posts.Tags").OrderBy("name").Get()
	require.NoError(t, err)
	assert.Equal(t, []string{"Alice"}, namesOf(authors), "only Alice has tagged posts")
}

func TestBelongsToHas(t *testing.T) {
	setupBlog(t)
	seedBlog(t)

	// Articles whose author is named Bob.
	articles, err := database.Query[Article]().
		WhereHas("Author", func(author *query.Builder) {
			author.Where("name", "Bob")
		}).Get()
	require.NoError(t, err)
	require.Len(t, articles, 1)
	assert.Equal(t, "Bob's", articles[0].Title)
}

func TestWhereHasUnknownRelationErrors(t *testing.T) {
	setupBlog(t)
	_, err := database.Query[Author]().Has("Nonsense").Get()
	assert.ErrorContains(t, err, `no relation "Nonsense"`)
}

func TestWhereHasComposesWithCountAndSoftDeletes(t *testing.T) {
	setupBlog(t)
	seedBlog(t)

	count, err := database.Query[Author]().Has("Posts").Count()
	require.NoError(t, err)
	assert.EqualValues(t, 2, count)

	exists, err := database.Query[Author]().Has("Profile").Exists()
	require.NoError(t, err)
	assert.True(t, exists)
}
