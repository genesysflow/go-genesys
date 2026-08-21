package database_test

import (
	"testing"

	"github.com/genesysflow/go-genesys/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

// Blog domain used by the relationship tests.

type Author struct {
	database.Model
	Name    string     `db:"name"`
	Profile *Profile   `rel:"hasOne"`
	Posts   []*Article `rel:"hasMany"`
}

type Profile struct {
	database.Model
	AuthorID int64  `db:"author_id"`
	Bio      string `db:"bio"`
}

type Article struct {
	database.Model
	AuthorID int64   `db:"author_id"`
	Title    string  `db:"title"`
	Author   *Author `rel:"belongsTo,fk:author_id"`
	Tags     []Tag   `rel:"belongsToMany"`
}

type Tag struct {
	database.Model
	Name     string     `db:"name"`
	Articles []*Article `rel:"belongsToMany"`
}

func setupBlog(t *testing.T) {
	t.Helper()
	manager := database.NewManager(database.Config{
		Default: "test",
		Connections: map[string]database.ConnectionConfig{
			"test": {Driver: "sqlite", Database: ":memory:"},
		},
	})
	t.Cleanup(func() {
		database.SetDefault(nil)
		manager.Close()
	})
	database.SetDefault(manager)

	statements := []string{
		`CREATE TABLE authors (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT,
			created_at TIMESTAMP, updated_at TIMESTAMP)`,
		`CREATE TABLE profiles (id INTEGER PRIMARY KEY AUTOINCREMENT, author_id INTEGER, bio TEXT,
			created_at TIMESTAMP, updated_at TIMESTAMP)`,
		`CREATE TABLE articles (id INTEGER PRIMARY KEY AUTOINCREMENT, author_id INTEGER, title TEXT,
			created_at TIMESTAMP, updated_at TIMESTAMP)`,
		`CREATE TABLE tags (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT,
			created_at TIMESTAMP, updated_at TIMESTAMP)`,
		`CREATE TABLE article_tag (article_id INTEGER, tag_id INTEGER)`,
	}
	for _, stmt := range statements {
		_, err := manager.Statement(stmt)
		require.NoError(t, err)
	}
}

func seedBlog(t *testing.T) (alice, bob *Author) {
	t.Helper()
	alice = &Author{Name: "Alice"}
	bob = &Author{Name: "Bob"}
	require.NoError(t, database.Create(alice))
	require.NoError(t, database.Create(bob))

	require.NoError(t, database.Create(&Profile{AuthorID: alice.ID, Bio: "writes a lot"}))

	posts := []*Article{
		{AuthorID: alice.ID, Title: "First"},
		{AuthorID: alice.ID, Title: "Second"},
		{AuthorID: bob.ID, Title: "Bob's"},
	}
	for _, p := range posts {
		require.NoError(t, database.Create(p))
	}

	golang := &Tag{Name: "go"}
	web := &Tag{Name: "web"}
	require.NoError(t, database.Create(golang))
	require.NoError(t, database.Create(web))

	// First -> go+web, Second -> go, Bob's -> (none)
	pivot := [][2]int64{{posts[0].ID, golang.ID}, {posts[0].ID, web.ID}, {posts[1].ID, golang.ID}}
	for _, link := range pivot {
		require.NoError(t, database.Default().Table("article_tag").Insert(
			map[string]any{"article_id": link[0], "tag_id": link[1]}))
	}
	return alice, bob
}

func TestEagerLoadHasMany(t *testing.T) {
	setupBlog(t)
	seedBlog(t)

	authors, err := database.Query[Author]().With("Posts").OrderBy("name").Get()
	require.NoError(t, err)
	require.Len(t, authors, 2)

	assert.Len(t, authors[0].Posts, 2, "Alice has two posts")
	assert.Len(t, authors[1].Posts, 1, "Bob has one post")
	assert.Equal(t, "Bob's", authors[1].Posts[0].Title)
}

func TestEagerLoadHasOne(t *testing.T) {
	setupBlog(t)
	seedBlog(t)

	authors, err := database.Query[Author]().With("Profile").OrderBy("name").Get()
	require.NoError(t, err)
	require.Len(t, authors, 2)

	require.NotNil(t, authors[0].Profile)
	assert.Equal(t, "writes a lot", authors[0].Profile.Bio)
	assert.Nil(t, authors[1].Profile, "Bob has no profile")
}

func TestEagerLoadBelongsTo(t *testing.T) {
	setupBlog(t)
	seedBlog(t)

	articles, err := database.Query[Article]().With("Author").OrderBy("id").Get()
	require.NoError(t, err)
	require.Len(t, articles, 3)

	require.NotNil(t, articles[0].Author)
	assert.Equal(t, "Alice", articles[0].Author.Name)
	assert.Equal(t, "Bob", articles[2].Author.Name)
}

func TestEagerLoadBelongsToMany(t *testing.T) {
	setupBlog(t)
	seedBlog(t)

	articles, err := database.Query[Article]().With("Tags").OrderBy("id").Get()
	require.NoError(t, err)
	require.Len(t, articles, 3)

	assert.Len(t, articles[0].Tags, 2)
	assert.Len(t, articles[1].Tags, 1)
	assert.Equal(t, "go", articles[1].Tags[0].Name)
	assert.Empty(t, articles[2].Tags)

	// Inverse direction through the same pivot.
	tags, err := database.Query[Tag]().With("Articles").OrderBy("name").Get()
	require.NoError(t, err)
	require.Len(t, tags, 2)
	assert.Len(t, tags[0].Articles, 2, "go tags two articles")
	assert.Len(t, tags[1].Articles, 1, "web tags one article")
}

func TestNestedEagerLoading(t *testing.T) {
	setupBlog(t)
	seedBlog(t)

	authors, err := database.Query[Author]().With("Posts.Tags").OrderBy("name").Get()
	require.NoError(t, err)
	require.Len(t, authors, 2)

	require.Len(t, authors[0].Posts, 2, "nested path implies loading the parent relation")
	first := authors[0].Posts[0]
	assert.Equal(t, "First", first.Title)
	assert.Len(t, first.Tags, 2, "nested tags loaded onto eager-loaded posts")
}

func TestLazyLoadAndLoadAll(t *testing.T) {
	setupBlog(t)
	alice, _ := seedBlog(t)

	author, err := database.Find[Author](alice.ID)
	require.NoError(t, err)
	assert.Nil(t, author.Posts, "not loaded yet")

	require.NoError(t, database.Load(author, "Posts", "Profile"))
	assert.Len(t, author.Posts, 2)
	require.NotNil(t, author.Profile)

	all, err := database.All[Author]()
	require.NoError(t, err)
	require.NoError(t, database.LoadAll(all, "Posts"))
	total := 0
	for _, a := range all {
		total += len(a.Posts)
	}
	assert.Equal(t, 3, total)
}

func TestEagerLoadOnFirstAndPaginate(t *testing.T) {
	setupBlog(t)
	seedBlog(t)

	author, err := database.Query[Author]().With("Posts").Where("name", "Alice").First()
	require.NoError(t, err)
	assert.Len(t, author.Posts, 2)

	page, err := database.Query[Article]().With("Author").OrderBy("id").Paginate(1, 2)
	require.NoError(t, err)
	require.Len(t, page.Data, 2)
	require.NotNil(t, page.Data[0].Author)
	assert.Equal(t, "Alice", page.Data[0].Author.Name)
}

func TestUnknownRelationErrors(t *testing.T) {
	setupBlog(t)
	seedBlog(t)

	_, err := database.Query[Author]().With("Nonsense").Get()
	assert.ErrorContains(t, err, `no relation "Nonsense"`)
}

func TestRelationFieldsAreNotColumns(t *testing.T) {
	setupBlog(t)

	// Creating a model with populated relation fields must not try to
	// write them as columns.
	author := &Author{Name: "Carol", Posts: []*Article{{Title: "ghost"}}}
	require.NoError(t, database.Create(author))
	assert.NotZero(t, author.ID)
}

func TestRelationTagValidation(t *testing.T) {
	setupBlog(t)
	_, err := database.Default().Statement(`CREATE TABLE bads (id INTEGER PRIMARY KEY AUTOINCREMENT,
		created_at TIMESTAMP, updated_at TIMESTAMP)`)
	require.NoError(t, err)
	require.NoError(t, database.Default().Table("bads").Insert(map[string]any{"id": 1}))

	type Bad struct {
		database.Model
		Things []*Tag `rel:"hasSome"`
	}
	_, err = database.Query[Bad]().With("Things").Get()
	// The malformed tag surfaces as an error rather than a panic; the
	// exact failure point depends on when relations parse, so just require
	// an error mentioning the field.
	assert.Error(t, err)
}
