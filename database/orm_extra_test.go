package database_test

import (
	"testing"

	"github.com/genesysflow/go-genesys/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

func seedTypedUsers(t *testing.T) {
	t.Helper()
	setupORM(t)
	for _, u := range []*User{
		{Name: "Alice", Email: "a@x.io", Age: 30, Active: true},
		{Name: "Bob", Email: "b@x.io", Age: 25, Active: true},
		{Name: "Carol", Email: "c@x.io", Age: 41, Active: false},
	} {
		require.NoError(t, database.Create(u))
	}
}

func TestModelQueryVariants(t *testing.T) {
	seedTypedUsers(t)

	// WhereIn / OrWhere
	users, err := database.Query[User]().WhereIn("name", "Alice", "Bob").OrderBy("name").Get()
	require.NoError(t, err)
	assert.Len(t, users, 2)

	users, err = database.Query[User]().Where("name", "Alice").OrWhere("name", "Carol").OrderBy("name").Get()
	require.NoError(t, err)
	assert.Len(t, users, 2)

	// WhereNull / WhereNotNull (email is always set)
	count, err := database.Query[User]().WhereNotNull("email").Count()
	require.NoError(t, err)
	assert.EqualValues(t, 3, count)
	none, err := database.Query[User]().WhereNull("email").Get()
	require.NoError(t, err)
	assert.Empty(t, none)

	// Latest / Limit / Offset
	latest, err := database.Query[User]().Latest("id").Limit(1).Get()
	require.NoError(t, err)
	require.Len(t, latest, 1)
	assert.Equal(t, "Carol", latest[0].Name)

	second, err := database.Query[User]().OrderBy("id").Offset(1).Limit(1).Get()
	require.NoError(t, err)
	require.Len(t, second, 1)
	assert.Equal(t, "Bob", second[0].Name)

	// Exists / OrderByDesc
	exists, err := database.Query[User]().Where("age", ">", 100).Exists()
	require.NoError(t, err)
	assert.False(t, exists)
	desc, err := database.Query[User]().OrderByDesc("age").First()
	require.NoError(t, err)
	assert.Equal(t, "Carol", desc.Name)

	// Typed Delete
	deleted, err := database.Query[User]().Where("active", false).Delete()
	require.NoError(t, err)
	assert.EqualValues(t, 1, deleted)

	// DeleteModel
	first, err := database.FirstWhere[User]("name", "Alice")
	require.NoError(t, err)
	require.NoError(t, database.DeleteModel(first))
	_, err = database.Find[User](first.ID)
	assert.ErrorIs(t, err, database.ErrNotFound)

	// Builder escape hatch shares state with the typed query.
	q := database.Query[User]().Where("name", "Bob")
	sqlStr, _ := q.Builder().ToSQL()
	assert.Contains(t, sqlStr, `"users"`)
}

func TestQueryOnExplicitExecutor(t *testing.T) {
	seedTypedUsers(t)
	conn := database.Default().Connection()

	users, err := database.QueryOn[User](conn.Driver(), conn).Where("age", ">=", 30).Get()
	require.NoError(t, err)
	assert.Len(t, users, 2)
}

func TestManagerTableEntryPoint(t *testing.T) {
	seedTypedUsers(t)

	count, err := database.Default().Table("users").Count()
	require.NoError(t, err)
	assert.EqualValues(t, 3, count)

	// TableOn against the raw connection.
	conn := database.Default().Connection()
	rows, err := database.TableOn(conn.Driver(), conn, "users").Where("name", "Bob").Get()
	require.NoError(t, err)
	assert.Len(t, rows, 1)
}

func TestUpdateWithoutIDFails(t *testing.T) {
	setupORM(t)
	user := &User{Name: "Ghost", Email: "g@x.io"}
	assert.ErrorContains(t, database.Update(user), "without an id")
	assert.ErrorIs(t, database.Update(&User{Model: database.Model{ID: 999}, Name: "X", Email: "x@x.io"}), database.ErrNotFound)
}
