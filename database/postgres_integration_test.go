package database_test

import (
	"testing"

	"github.com/genesysflow/go-genesys/database"
	"github.com/genesysflow/go-genesys/query"
	"github.com/genesysflow/go-genesys/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	_ "github.com/lib/pq"
)

// TestPostgresORMIntegration exercises the ORM end-to-end against a
// real PostgreSQL server, validating the $n placeholder grammar,
// RETURNING-based inserts, timestamp round-trips, relations, soft
// deletes, pivot writes, and pagination outside of sqlite. It uses a
// CI services container (GENESYS_TEST_PG_* env) or a docker container,
// and skips when neither is available.
func TestPostgresORMIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping postgres integration in -short mode")
	}
	pc := testutil.PostgresForTests(t)

	manager := database.NewManager(database.Config{
		Default: "pg",
		Connections: map[string]database.ConnectionConfig{
			"pg": {
				Driver:   "pgsql",
				Host:     pc.Host,
				Port:     pc.Port,
				Database: pc.Database,
				Username: pc.Username,
				Password: pc.Password,
				SSLMode:  "disable",
			},
		},
	})
	t.Cleanup(func() {
		database.SetDefault(nil)
		manager.Close()
	})
	database.SetDefault(manager)

	conn := manager.Connection()
	require.NoError(t, conn.Ping(), "postgres must be reachable")

	for _, ddl := range []string{
		`DROP TABLE IF EXISTS article_tag, profiles, articles, tags, authors, documents CASCADE`,
		`CREATE TABLE authors (id SERIAL PRIMARY KEY, name TEXT NOT NULL,
			created_at TIMESTAMPTZ, updated_at TIMESTAMPTZ)`,
		`CREATE TABLE profiles (id SERIAL PRIMARY KEY, author_id BIGINT NOT NULL, bio TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMPTZ, updated_at TIMESTAMPTZ)`,
		`CREATE TABLE articles (id SERIAL PRIMARY KEY, author_id BIGINT NOT NULL DEFAULT 0, title TEXT NOT NULL,
			created_at TIMESTAMPTZ, updated_at TIMESTAMPTZ)`,
		`CREATE TABLE tags (id SERIAL PRIMARY KEY, name TEXT NOT NULL,
			created_at TIMESTAMPTZ, updated_at TIMESTAMPTZ)`,
		`CREATE TABLE article_tag (article_id BIGINT NOT NULL, tag_id BIGINT NOT NULL)`,
		`CREATE TABLE documents (id SERIAL PRIMARY KEY, title TEXT NOT NULL,
			created_at TIMESTAMPTZ, updated_at TIMESTAMPTZ, deleted_at TIMESTAMPTZ)`,
	} {
		_, err := manager.Statement(ddl)
		require.NoError(t, err)
	}

	t.Run("CRUDAndDirtyTracking", func(t *testing.T) {
		author := &Author{Name: "Ada"}
		require.NoError(t, database.Create(author))
		assert.NotZero(t, author.ID, "RETURNING id filled the primary key")
		assert.False(t, author.CreatedAt.IsZero())

		fetched, err := database.Find[Author](author.ID)
		require.NoError(t, err)
		assert.Equal(t, "Ada", fetched.Name)
		assert.False(t, database.IsDirty(fetched))

		fetched.Name = "Ada Lovelace"
		assert.Equal(t, map[string]any{"name": "Ada Lovelace"}, database.GetDirty(fetched))
		require.NoError(t, database.Update(fetched))

		again, err := database.Find[Author](author.ID)
		require.NoError(t, err)
		assert.Equal(t, "Ada Lovelace", again.Name)
	})

	t.Run("RelationsAndWhereHas", func(t *testing.T) {
		bob := &Author{Name: "Bob"}
		require.NoError(t, database.Create(bob))
		require.NoError(t, database.CreateFor(bob, "Profile", &Profile{Bio: "writes Go"}))
		first := &Article{Title: "Postgres First"}
		second := &Article{Title: "Postgres Second"}
		require.NoError(t, database.CreateFor(bob, "Posts", first))
		require.NoError(t, database.CreateFor(bob, "Posts", second))

		golang := &Tag{Name: "go"}
		web := &Tag{Name: "web"}
		require.NoError(t, database.Create(golang))
		require.NoError(t, database.Create(web))
		require.NoError(t, database.Attach(first, "Tags", golang, web))
		require.NoError(t, database.Attach(second, "Tags", golang))

		// Nested eager loading in one pass.
		authors, err := database.Query[Author]().
			Where("name", "Bob").With("Profile", "Posts.Tags").Get()
		require.NoError(t, err)
		require.Len(t, authors, 1)
		require.NotNil(t, authors[0].Profile)
		require.Len(t, authors[0].Posts, 2)
		assert.Len(t, authors[0].Posts[0].Tags, 2)

		// WhereHas through the pivot with a constraint.
		tagged, err := database.Query[Article]().
			WhereHas("Tags", func(tags *query.Builder) {
				tags.Where("tags.name", "web")
			}).Get()
		require.NoError(t, err)
		require.Len(t, tagged, 1)
		assert.Equal(t, "Postgres First", tagged[0].Title)

		// Nested existence path.
		withTaggedPosts, err := database.Query[Author]().Has("Posts.Tags").Get()
		require.NoError(t, err)
		require.Len(t, withTaggedPosts, 1)
		assert.Equal(t, "Bob", withTaggedPosts[0].Name)

		// Sync then verify the pivot shrank.
		require.NoError(t, database.Sync(first, "Tags", web))
		require.NoError(t, database.Load(first, "Tags"))
		require.Len(t, first.Tags, 1)
		assert.Equal(t, "web", first.Tags[0].Name)
	})

	t.Run("SoftDeletes", func(t *testing.T) {
		keep := &Document{Title: "keep"}
		trash := &Document{Title: "trash"}
		require.NoError(t, database.Create(keep))
		require.NoError(t, database.Create(trash))
		require.NoError(t, database.Delete[Document](trash.ID))

		live, err := database.Query[Document]().Count()
		require.NoError(t, err)
		assert.EqualValues(t, 1, live)

		all, err := database.Query[Document]().WithTrashed().Count()
		require.NoError(t, err)
		assert.EqualValues(t, 2, all)

		require.NoError(t, database.Restore[Document](trash.ID))
		restored, err := database.Find[Document](trash.ID)
		require.NoError(t, err)
		assert.Nil(t, restored.DeletedAt)
	})

	t.Run("CursorPaginationAndChunk", func(t *testing.T) {
		for i := 0; i < 5; i++ {
			require.NoError(t, database.Create(&Tag{Name: "page-tag"}))
		}

		var seen int
		cursor := ""
		for {
			page, err := database.Query[Tag]().Where("name", "page-tag").CursorPaginate(2, cursor)
			require.NoError(t, err)
			seen += len(page.Data)
			if page.NextCursor == "" {
				break
			}
			cursor = page.NextCursor
		}
		assert.Equal(t, 5, seen)

		var chunked int
		require.NoError(t, database.Query[Tag]().Where("name", "page-tag").Chunk(2, func(tags []Tag) error {
			chunked += len(tags)
			return nil
		}))
		assert.Equal(t, 5, chunked)
	})
}
