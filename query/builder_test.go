package query_test

import (
	"database/sql"
	"testing"

	"github.com/genesysflow/go-genesys/query"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

func TestCompileSelectBasic(t *testing.T) {
	b := query.New("sqlite", nil).Table("users")
	sqlStr, bindings := b.ToSQL()
	assert.Equal(t, `SELECT * FROM "users"`, sqlStr)
	assert.Empty(t, bindings)
}

func TestCompileSelectWheres(t *testing.T) {
	b := query.New("sqlite", nil).Table("users").
		Select("id", "name").
		Where("age", ">", 18).
		OrWhere("admin", true).
		WhereIn("role", "a", "b").
		WhereNull("deleted_at").
		OrderByDesc("created_at").
		Limit(10).Offset(5)

	sqlStr, bindings := b.ToSQL()
	assert.Equal(t,
		`SELECT "id", "name" FROM "users" WHERE "age" > ? OR "admin" = ? AND "role" IN (?, ?) AND "deleted_at" IS NULL ORDER BY "created_at" DESC LIMIT 10 OFFSET 5`,
		sqlStr)
	assert.Equal(t, []any{18, true, "a", "b"}, bindings)
}

func TestCompileSelectPostgresPlaceholders(t *testing.T) {
	b := query.New("pgsql", nil).Table("users").
		Where("age", ">", 18).
		WhereIn("role", "a", "b").
		WhereRaw("lower(email) = ?", "x@y.z")

	sqlStr, bindings := b.ToSQL()
	assert.Equal(t,
		`SELECT * FROM "users" WHERE "age" > $1 AND "role" IN ($2, $3) AND lower(email) = $4`,
		sqlStr)
	assert.Len(t, bindings, 4)
}

func TestCompileNestedWhere(t *testing.T) {
	b := query.New("sqlite", nil).Table("users").
		Where("active", true).
		WhereGroup(func(q *query.Builder) {
			q.Where("age", "<", 13).OrWhere("age", ">", 65)
		})

	sqlStr, bindings := b.ToSQL()
	assert.Equal(t,
		`SELECT * FROM "users" WHERE "active" = ? AND ("age" < ? OR "age" > ?)`,
		sqlStr)
	assert.Equal(t, []any{true, 13, 65}, bindings)
}

func TestCompileJoinsGroupsHaving(t *testing.T) {
	b := query.New("mysql", nil).Table("users").
		Select("users.name").SelectRaw("COUNT(posts.id) as post_count").
		Join("posts", "users.id", "=", "posts.user_id").
		GroupBy("users.name").
		Having("post_count", ">", 3)

	sqlStr, bindings := b.ToSQL()
	assert.Equal(t,
		"SELECT `users`.`name`, COUNT(posts.id) as post_count FROM `users` INNER JOIN `posts` ON `users`.`id` = `posts`.`user_id` GROUP BY `users`.`name` HAVING `post_count` > ?",
		sqlStr)
	assert.Equal(t, []any{3}, bindings)
}

func TestEmptyWhereIn(t *testing.T) {
	sqlStr, bindings := query.New("sqlite", nil).Table("users").WhereIn("id").ToSQL()
	assert.Equal(t, `SELECT * FROM "users" WHERE 1 = 0`, sqlStr)
	assert.Empty(t, bindings)
}

// sqliteExecutor adapts *sql.DB to the query.Executor interface.
type sqliteExecutor struct{ db *sql.DB }

func (e *sqliteExecutor) Query(q string, bindings ...any) (*sql.Rows, error) {
	return e.db.Query(q, bindings...)
}
func (e *sqliteExecutor) QueryRow(q string, bindings ...any) *sql.Row {
	return e.db.QueryRow(q, bindings...)
}
func (e *sqliteExecutor) Exec(q string, bindings ...any) (sql.Result, error) {
	return e.db.Exec(q, bindings...)
}

func newTestDB(t *testing.T) *sqliteExecutor {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	_, err = db.Exec(`CREATE TABLE users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		email TEXT NOT NULL,
		age INTEGER NOT NULL DEFAULT 0,
		active INTEGER NOT NULL DEFAULT 1
	)`)
	require.NoError(t, err)
	return &sqliteExecutor{db: db}
}

func seedUsers(t *testing.T, exec *sqliteExecutor) {
	t.Helper()
	b := query.New("sqlite", exec).Table("users")
	err := b.Insert(
		map[string]any{"name": "Alice", "email": "alice@example.com", "age": 30, "active": 1},
		map[string]any{"name": "Bob", "email": "bob@example.com", "age": 25, "active": 1},
		map[string]any{"name": "Carol", "email": "carol@example.com", "age": 41, "active": 0},
	)
	require.NoError(t, err)
}

func TestIntegrationCRUD(t *testing.T) {
	exec := newTestDB(t)
	seedUsers(t, exec)

	table := func() *query.Builder { return query.New("sqlite", exec).Table("users") }

	rows, err := table().OrderBy("name").Get()
	require.NoError(t, err)
	require.Len(t, rows, 3)
	assert.Equal(t, "Alice", rows[0]["name"])

	first, err := table().Where("age", ">", 26).OrderBy("age").First()
	require.NoError(t, err)
	assert.Equal(t, "Alice", first["name"])

	_, err = table().Where("age", ">", 100).First()
	assert.ErrorIs(t, err, query.ErrNoRows)

	count, err := table().Where("active", 1).Count()
	require.NoError(t, err)
	assert.EqualValues(t, 2, count)

	avg, err := table().Avg("age")
	require.NoError(t, err)
	assert.InDelta(t, 32.0, avg, 0.01)

	names, err := table().OrderBy("name").Pluck("name")
	require.NoError(t, err)
	assert.Equal(t, []any{"Alice", "Bob", "Carol"}, names)

	id, err := table().InsertGetID(map[string]any{"name": "Dave", "email": "dave@example.com", "age": 20, "active": 1})
	require.NoError(t, err)
	assert.EqualValues(t, 4, id)

	affected, err := table().Where("name", "Dave").Update(map[string]any{"age": 21})
	require.NoError(t, err)
	assert.EqualValues(t, 1, affected)

	dave, err := table().Find(id)
	require.NoError(t, err)
	assert.EqualValues(t, 21, dave["age"])

	_, err = table().Where("name", "Dave").Increment("age", 4)
	require.NoError(t, err)
	age, err := table().Where("name", "Dave").Value("age")
	require.NoError(t, err)
	assert.EqualValues(t, 25, age)

	deleted, err := table().Where("active", 0).Delete()
	require.NoError(t, err)
	assert.EqualValues(t, 1, deleted)

	exists, err := table().Where("name", "Carol").Exists()
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestIntegrationPagination(t *testing.T) {
	exec := newTestDB(t)
	b := query.New("sqlite", exec).Table("users")
	for i := 0; i < 25; i++ {
		require.NoError(t, b.Insert(map[string]any{
			"name": "User", "email": "u@example.com", "age": i, "active": 1,
		}))
	}

	page, err := query.New("sqlite", exec).Table("users").OrderBy("id").Paginate(2, 10)
	require.NoError(t, err)
	assert.EqualValues(t, 25, page.Total)
	assert.Equal(t, 10, page.PerPage)
	assert.Equal(t, 2, page.CurrentPage)
	assert.Equal(t, 3, page.LastPage)
	assert.Equal(t, 11, page.From)
	assert.Equal(t, 20, page.To)
	assert.Len(t, page.Data, 10)

	simple, err := query.New("sqlite", exec).Table("users").OrderBy("id").SimplePaginate(3, 10)
	require.NoError(t, err)
	assert.Len(t, simple.Data, 5)
	assert.False(t, simple.HasMore)
}
