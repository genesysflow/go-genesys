package database_test

import (
	"testing"
	"time"

	"github.com/genesysflow/go-genesys/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

type User struct {
	database.Model
	Name   string `db:"name" json:"name"`
	Email  string `db:"email" json:"email"`
	Age    int    `db:"age" json:"age"`
	Active bool   `db:"active" json:"active"`
}

type BlogPost struct {
	database.Model
	Title string `db:"title"`
}

func setupORM(t *testing.T) {
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

	_, err := manager.Statement(`CREATE TABLE users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		email TEXT NOT NULL,
		age INTEGER NOT NULL DEFAULT 0,
		active INTEGER NOT NULL DEFAULT 1,
		created_at TIMESTAMP,
		updated_at TIMESTAMP
	)`)
	require.NoError(t, err)
}

func TestTableNameInference(t *testing.T) {
	assert.Equal(t, "users", database.TableNameFor[User]())
	assert.Equal(t, "blog_posts", database.TableNameFor[BlogPost]())
}

func TestCreateFindUpdateDelete(t *testing.T) {
	setupORM(t)

	user := &User{Name: "Alice", Email: "alice@example.com", Age: 30, Active: true}
	require.NoError(t, database.Create(user))
	assert.EqualValues(t, 1, user.ID)
	assert.False(t, user.CreatedAt.IsZero())
	assert.False(t, user.UpdatedAt.IsZero())

	found, err := database.Find[User](user.ID)
	require.NoError(t, err)
	assert.Equal(t, "Alice", found.Name)
	assert.Equal(t, 30, found.Age)
	assert.True(t, found.Active)
	assert.False(t, found.CreatedAt.IsZero())

	_, err = database.Find[User](999)
	assert.ErrorIs(t, err, database.ErrNotFound)

	found.Age = 31
	require.NoError(t, database.Update(found))
	again, err := database.Find[User](user.ID)
	require.NoError(t, err)
	assert.Equal(t, 31, again.Age)

	require.NoError(t, database.Delete[User](user.ID))
	_, err = database.Find[User](user.ID)
	assert.ErrorIs(t, err, database.ErrNotFound)
	assert.ErrorIs(t, database.Delete[User](user.ID), database.ErrNotFound)
}

func TestSaveInsertsThenUpdates(t *testing.T) {
	setupORM(t)

	user := &User{Name: "Bob", Email: "bob@example.com"}
	require.NoError(t, database.Save(user))
	assert.EqualValues(t, 1, user.ID)

	user.Name = "Bobby"
	require.NoError(t, database.Save(user))

	all, err := database.All[User]()
	require.NoError(t, err)
	require.Len(t, all, 1)
	assert.Equal(t, "Bobby", all[0].Name)
}

func TestTypedQuery(t *testing.T) {
	setupORM(t)

	for _, u := range []*User{
		{Name: "Alice", Email: "a@x.io", Age: 30, Active: true},
		{Name: "Bob", Email: "b@x.io", Age: 25, Active: true},
		{Name: "Carol", Email: "c@x.io", Age: 41, Active: false},
	} {
		require.NoError(t, database.Create(u))
	}

	adults, err := database.Query[User]().Where("age", ">=", 30).OrderBy("name").Get()
	require.NoError(t, err)
	require.Len(t, adults, 2)
	assert.Equal(t, "Alice", adults[0].Name)
	assert.Equal(t, "Carol", adults[1].Name)

	first, err := database.FirstWhere[User]("email", "b@x.io")
	require.NoError(t, err)
	assert.Equal(t, "Bob", first.Name)

	count, err := database.Query[User]().Where("active", true).Count()
	require.NoError(t, err)
	assert.EqualValues(t, 2, count)

	affected, err := database.Query[User]().Where("active", false).Update(map[string]any{"active": true})
	require.NoError(t, err)
	assert.EqualValues(t, 1, affected)

	page, err := database.Query[User]().OrderBy("id").Paginate(1, 2)
	require.NoError(t, err)
	assert.EqualValues(t, 3, page.Total)
	assert.Equal(t, 2, page.LastPage)
	require.Len(t, page.Data, 2)
	assert.Equal(t, "Alice", page.Data[0].Name)
}

func TestTimestampsRoundTrip(t *testing.T) {
	setupORM(t)

	before := time.Now().UTC().Add(-time.Minute)
	user := &User{Name: "Tim", Email: "t@x.io"}
	require.NoError(t, database.Create(user))

	found, err := database.Find[User](user.ID)
	require.NoError(t, err)
	assert.True(t, found.CreatedAt.After(before), "created_at should round-trip through the driver")
}
