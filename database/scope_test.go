package database_test

import (
	"errors"
	"testing"

	"github.com/genesysflow/go-genesys/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

func TestTransactionRollsBackOnError(t *testing.T) {
	setupBlog(t)

	err := database.WithinTransaction(func(tx *database.TxScope) error {
		author := &Author{Name: "Doomed"}
		if err := database.Create(author, tx); err != nil {
			return err
		}
		if err := database.CreateFor(author, "Posts", &Article{Title: "Doomed too"}, tx); err != nil {
			return err
		}

		// Inside the transaction the rows are visible through the scope...
		count, err := database.Query[Author](tx).Count()
		if err != nil {
			return err
		}
		assert.EqualValues(t, 1, count)

		return errors.New("business rule failed")
	})
	require.ErrorContains(t, err, "business rule failed")

	// ...but after the rollback nothing persisted.
	authors, err := database.All[Author]()
	require.NoError(t, err)
	assert.Empty(t, authors, "rollback removed the author")
	articles, err := database.All[Article]()
	require.NoError(t, err)
	assert.Empty(t, articles, "rollback removed the child too")
}

func TestTransactionCommits(t *testing.T) {
	setupBlog(t)

	var authorID int64
	err := database.WithinTransaction(func(tx *database.TxScope) error {
		author := &Author{Name: "Kept"}
		if err := database.Create(author, tx); err != nil {
			return err
		}
		authorID = author.ID

		tag := &Tag{Name: "tx-tag"}
		if err := database.Create(tag, tx); err != nil {
			return err
		}
		article := &Article{Title: "Kept article"}
		if err := database.CreateFor(author, "Posts", article, tx); err != nil {
			return err
		}
		return database.AttachScoped(tx, article, "Tags", tag)
	})
	require.NoError(t, err)

	// Everything is visible outside the transaction after commit.
	author, err := database.Find[Author](authorID)
	require.NoError(t, err)
	require.NoError(t, database.Load(author, "Posts.Tags"))
	require.Len(t, author.Posts, 1)
	require.Len(t, author.Posts[0].Tags, 1)
	assert.Equal(t, "tx-tag", author.Posts[0].Tags[0].Name)
}

func TestTransactionRollsBackOnPanic(t *testing.T) {
	setupBlog(t)

	assert.Panics(t, func() {
		database.WithinTransaction(func(tx *database.TxScope) error {
			if err := database.Create(&Author{Name: "Panicked"}, tx); err != nil {
				return err
			}
			panic("boom")
		})
	})

	authors, err := database.All[Author]()
	require.NoError(t, err)
	assert.Empty(t, authors, "panic rolled the write back")
}

func TestScopedUpdateDeleteAndSoftDeletes(t *testing.T) {
	_, trashed := setupDocuments(t) // 1 live "kept", 1 trashed

	err := database.WithinTransaction(func(tx *database.TxScope) error {
		doc, err := database.Query[Document](tx).Where("title", "kept").First()
		if err != nil {
			return err
		}
		doc.Title = "kept-renamed"
		if err := database.Update(doc, tx); err != nil {
			return err
		}
		if err := database.Restore[Document](trashed.ID, tx); err != nil {
			return err
		}
		return errors.New("abort")
	})
	require.Error(t, err)

	// Both the rename and the restore rolled back.
	_, err = database.Query[Document]().Where("title", "kept-renamed").First()
	assert.ErrorIs(t, err, database.ErrNotFound)
	live, err := database.Query[Document]().Count()
	require.NoError(t, err)
	assert.EqualValues(t, 1, live, "restore rolled back, trashed stays trashed")
}

func TestFirstOrCreate(t *testing.T) {
	setupBlog(t)

	first, err := database.FirstOrCreate(map[string]any{"name": "Ada"}, &Author{Name: "Ada"})
	require.NoError(t, err)
	assert.NotZero(t, first.ID)

	// A second call finds instead of creating.
	again, err := database.FirstOrCreate(map[string]any{"name": "Ada"}, &Author{Name: "Ada"})
	require.NoError(t, err)
	assert.Equal(t, first.ID, again.ID)

	count, err := database.Query[Author]().Count()
	require.NoError(t, err)
	assert.EqualValues(t, 1, count)
}

func TestUpdateOrCreate(t *testing.T) {
	setupBlog(t)

	created, err := database.UpdateOrCreate(map[string]any{"name": "Grace"}, &Author{Name: "Grace"})
	require.NoError(t, err)
	require.NotZero(t, created.ID)
	originalCreatedAt := created.CreatedAt

	// Second call updates the same row instead of inserting.
	updated, err := database.UpdateOrCreate(map[string]any{"name": "Grace"},
		&Author{Name: "Grace"})
	require.NoError(t, err)
	assert.Equal(t, created.ID, updated.ID)
	assert.Equal(t, originalCreatedAt, updated.CreatedAt, "creation time preserved")

	count, err := database.Query[Author]().Count()
	require.NoError(t, err)
	assert.EqualValues(t, 1, count)

	// It also works inside a transaction scope.
	err = database.WithinTransaction(func(tx *database.TxScope) error {
		_, err := database.UpdateOrCreate(map[string]any{"name": "TxOnly"}, &Author{Name: "TxOnly"}, tx)
		if err != nil {
			return err
		}
		return errors.New("abort")
	})
	require.Error(t, err)
	_, err = database.FirstWhere[Author]("name", "TxOnly")
	assert.ErrorIs(t, err, database.ErrNotFound)
}
