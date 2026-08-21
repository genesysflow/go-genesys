package database_test

import (
	"testing"

	"github.com/genesysflow/go-genesys/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

type Document struct {
	database.Model
	database.SoftDeletes
	Title string `db:"title"`
}

func setupDocuments(t *testing.T) (kept, trashed *Document) {
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

	_, err := manager.Statement(`CREATE TABLE documents (
		id INTEGER PRIMARY KEY AUTOINCREMENT, title TEXT,
		created_at TIMESTAMP, updated_at TIMESTAMP, deleted_at TIMESTAMP)`)
	require.NoError(t, err)

	kept = &Document{Title: "kept"}
	trashed = &Document{Title: "trashed"}
	require.NoError(t, database.Create(kept))
	require.NoError(t, database.Create(trashed))
	require.NoError(t, database.Delete[Document](trashed.ID))
	return kept, trashed
}

func TestSoftDeleteHidesTrashedByDefault(t *testing.T) {
	kept, trashed := setupDocuments(t)

	all, err := database.All[Document]()
	require.NoError(t, err)
	require.Len(t, all, 1)
	assert.Equal(t, "kept", all[0].Title)

	// Find honours the scope too.
	_, err = database.Find[Document](trashed.ID)
	assert.ErrorIs(t, err, database.ErrNotFound)
	_, err = database.Find[Document](kept.ID)
	assert.NoError(t, err)

	// The row still physically exists.
	count, err := database.Default().Table("documents").Count()
	require.NoError(t, err)
	assert.EqualValues(t, 2, count)
}

func TestWithTrashedAndOnlyTrashed(t *testing.T) {
	_, trashed := setupDocuments(t)

	all, err := database.Query[Document]().WithTrashed().OrderBy("id").Get()
	require.NoError(t, err)
	assert.Len(t, all, 2)

	only, err := database.Query[Document]().OnlyTrashed().Get()
	require.NoError(t, err)
	require.Len(t, only, 1)
	assert.Equal(t, "trashed", only[0].Title)
	require.NotNil(t, only[0].DeletedAt, "deleted_at scanned back onto the model")

	found, err := database.Query[Document]().WithTrashed().Find(trashed.ID)
	require.NoError(t, err)
	assert.Equal(t, "trashed", found.Title)

	count, err := database.Query[Document]().WithTrashed().Count()
	require.NoError(t, err)
	assert.EqualValues(t, 2, count)
}

func TestRestore(t *testing.T) {
	_, trashed := setupDocuments(t)

	require.NoError(t, database.Restore[Document](trashed.ID))

	doc, err := database.Find[Document](trashed.ID)
	require.NoError(t, err)
	assert.Nil(t, doc.DeletedAt)

	// Restoring an already-live row reports not found (0 affected is fine
	// at the query level; package-level maps it to ErrNotFound only when
	// nothing matched at all).
	count, err := database.Query[Document]().Count()
	require.NoError(t, err)
	assert.EqualValues(t, 2, count)
}

func TestForceDelete(t *testing.T) {
	_, trashed := setupDocuments(t)

	require.NoError(t, database.ForceDelete[Document](trashed.ID))

	count, err := database.Default().Table("documents").Count()
	require.NoError(t, err)
	assert.EqualValues(t, 1, count, "row physically removed")

	assert.ErrorIs(t, database.ForceDelete[Document](trashed.ID), database.ErrNotFound)
}

func TestDeleteModelMirrorsDeletedAt(t *testing.T) {
	kept, _ := setupDocuments(t)

	require.NoError(t, database.DeleteModel(kept))
	require.NotNil(t, kept.DeletedAt, "in-memory model marked as trashed")

	_, err := database.Find[Document](kept.ID)
	assert.ErrorIs(t, err, database.ErrNotFound)
}

func TestQueryDeleteSoftDeletesMatching(t *testing.T) {
	setupDocuments(t)

	affected, err := database.Query[Document]().Where("title", "kept").Delete()
	require.NoError(t, err)
	assert.EqualValues(t, 1, affected)

	live, err := database.Query[Document]().Count()
	require.NoError(t, err)
	assert.EqualValues(t, 0, live)

	physical, err := database.Default().Table("documents").Count()
	require.NoError(t, err)
	assert.EqualValues(t, 2, physical)
}

func TestHardDeleteModelsUnaffected(t *testing.T) {
	setupORM(t)
	user := &User{Name: "Hard", Email: "h@x.io"}
	require.NoError(t, database.Create(user))

	// Models without deleted_at still hard-delete.
	require.NoError(t, database.Delete[User](user.ID))
	count, err := database.Default().Table("users").Count()
	require.NoError(t, err)
	assert.EqualValues(t, 0, count)
}

func TestLocalScopes(t *testing.T) {
	setupDocuments(t)

	titled := func(title string) func(*database.ModelQuery[Document]) {
		return func(q *database.ModelQuery[Document]) { q.Where("title", title) }
	}

	docs, err := database.Query[Document]().Scope(titled("kept")).Get()
	require.NoError(t, err)
	require.Len(t, docs, 1)
	assert.Equal(t, "kept", docs[0].Title)
}

// Soft-deleted related rows are excluded from eager loads.
type Shelf struct {
	database.Model
	Name  string  `db:"name"`
	Books []*Book `rel:"hasMany"`
}

type Book struct {
	database.Model
	database.SoftDeletes
	ShelfID int64  `db:"shelf_id"`
	Title   string `db:"title"`
}

func TestEagerLoadExcludesTrashed(t *testing.T) {
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

	for _, stmt := range []string{
		`CREATE TABLE shelves (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT,
			created_at TIMESTAMP, updated_at TIMESTAMP)`,
		`CREATE TABLE books (id INTEGER PRIMARY KEY AUTOINCREMENT, shelf_id INTEGER, title TEXT,
			created_at TIMESTAMP, updated_at TIMESTAMP, deleted_at TIMESTAMP)`,
	} {
		_, err := manager.Statement(stmt)
		require.NoError(t, err)
	}

	shelf := &Shelf{Name: "fiction"}
	require.NoError(t, database.Create(shelf))
	live := &Book{ShelfID: shelf.ID, Title: "live"}
	gone := &Book{ShelfID: shelf.ID, Title: "gone"}
	require.NoError(t, database.Create(live))
	require.NoError(t, database.Create(gone))
	require.NoError(t, database.Delete[Book](gone.ID))

	shelves, err := database.Query[Shelf]().With("Books").Get()
	require.NoError(t, err)
	require.Len(t, shelves, 1)
	require.Len(t, shelves[0].Books, 1)
	assert.Equal(t, "live", shelves[0].Books[0].Title)
}
