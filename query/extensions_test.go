package query_test

import (
	"database/sql"
	"testing"

	"github.com/genesysflow/go-genesys/query"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

type sqliteExec struct{ db *sql.DB }

func (e *sqliteExec) Query(q string, b ...any) (*sql.Rows, error) { return e.db.Query(q, b...) }
func (e *sqliteExec) QueryRow(q string, b ...any) *sql.Row        { return e.db.QueryRow(q, b...) }
func (e *sqliteExec) Exec(q string, b ...any) (sql.Result, error) { return e.db.Exec(q, b...) }

func newExtDB(t *testing.T) *sqliteExec {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	for _, stmt := range []string{
		`CREATE TABLE items (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL, meta TEXT NOT NULL DEFAULT '{}')`,
		`CREATE TABLE archived (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL)`,
	} {
		_, err := db.Exec(stmt)
		require.NoError(t, err)
	}
	return &sqliteExec{db: db}
}

func TestLockClausesPerDriver(t *testing.T) {
	sqlStr, _ := query.New("pgsql", nil).Table("users").Where("id", 1).LockForUpdate().ToSQL()
	assert.Contains(t, sqlStr, "FOR UPDATE")

	sqlStr, _ = query.New("pgsql", nil).Table("users").SharedLock().ToSQL()
	assert.Contains(t, sqlStr, "FOR SHARE")

	sqlStr, _ = query.New("mysql", nil).Table("users").SharedLock().ToSQL()
	assert.Contains(t, sqlStr, "LOCK IN SHARE MODE")

	// SQLite locks the whole database; the clause would be a syntax error.
	sqlStr, _ = query.New("sqlite", nil).Table("users").LockForUpdate().ToSQL()
	assert.NotContains(t, sqlStr, "FOR UPDATE")
}

func TestUnionCombinesAndOrders(t *testing.T) {
	exec := newExtDB(t)
	seed := []string{"beta", "delta"}
	for _, n := range seed {
		_, err := exec.Exec(`INSERT INTO items (name) VALUES (?)`, n)
		require.NoError(t, err)
	}
	_, err := exec.Exec(`INSERT INTO archived (name) VALUES ('alpha'), ('beta')`)
	require.NoError(t, err)

	other := query.New("sqlite", exec).Table("archived").Select("name")
	rows, err := query.New("sqlite", exec).Table("items").Select("name").
		Union(other).OrderBy("name").Get()
	require.NoError(t, err)

	names := make([]string, len(rows))
	for i, r := range rows {
		names[i] = r["name"].(string)
	}
	assert.Equal(t, []string{"alpha", "beta", "delta"}, names, "UNION dedupes and ORDER BY applies to the whole result")

	all, err := query.New("sqlite", exec).Table("items").Select("name").
		UnionAll(query.New("sqlite", exec).Table("archived").Select("name")).Get()
	require.NoError(t, err)
	assert.Len(t, all, 4, "UNION ALL keeps duplicates")
}

func TestUnionPlaceholderNumberingOnPostgres(t *testing.T) {
	other := query.New("pgsql", nil).Table("archived").Select("name").Where("name", "x")
	sqlStr, bindings := query.New("pgsql", nil).Table("items").Select("name").
		Where("name", "a").Union(other).ToSQL()
	assert.Contains(t, sqlStr, "$1")
	assert.Contains(t, sqlStr, "$2", "union bindings renumber after the primary query's")
	assert.NotContains(t, sqlStr[len(sqlStr)/2:], "$1", "the union half must not restart at $1")
	assert.Len(t, bindings, 2)
}

func TestWhereJSON(t *testing.T) {
	exec := newExtDB(t)
	_, err := exec.Exec(`INSERT INTO items (name, meta) VALUES
		('red-item', '{"color":"red","specs":{"weight":12}}'),
		('blue-item', '{"color":"blue","specs":{"weight":3}}')`)
	require.NoError(t, err)

	rows, err := query.New("sqlite", exec).Table("items").WhereJSON("meta", "color", "red").Get()
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "red-item", rows[0]["name"])

	heavy, err := query.New("sqlite", exec).Table("items").WhereJSON("meta", "specs.weight", ">", 5).Get()
	require.NoError(t, err)
	require.Len(t, heavy, 1)
	assert.Equal(t, "red-item", heavy[0]["name"])

	// Path injection is rejected loudly.
	assert.Panics(t, func() {
		query.New("sqlite", exec).Table("items").WhereJSON("meta", "a') OR 1=1 --", "x")
	})

	// Postgres renders the #>> path form.
	pgSQL, _ := query.New("pgsql", nil).Table("items").WhereJSON("meta", "specs.weight", 1).ToSQL()
	assert.Contains(t, pgSQL, `"meta" #>> '{specs,weight}'`)
}

func TestCrossJoin(t *testing.T) {
	exec := newExtDB(t)
	_, err := exec.Exec(`INSERT INTO items (name) VALUES ('a'), ('b')`)
	require.NoError(t, err)
	_, err = exec.Exec(`INSERT INTO archived (name) VALUES ('x'), ('y'), ('z')`)
	require.NoError(t, err)

	rows, err := query.New("sqlite", exec).Table("items").
		Select("items.name").CrossJoin("archived").Get()
	require.NoError(t, err)
	assert.Len(t, rows, 6, "cartesian product")
}

func TestBuilderUpsert(t *testing.T) {
	exec := newExtDB(t)
	_, err := exec.Exec(`CREATE TABLE prices (code TEXT PRIMARY KEY, amount INTEGER NOT NULL)`)
	require.NoError(t, err)

	q := func() *query.Builder { return query.New("sqlite", exec).Table("prices") }
	_, err = q().Upsert([]map[string]any{
		{"code": "A", "amount": 1},
		{"code": "B", "amount": 2},
	}, []string{"code"}, nil)
	require.NoError(t, err)

	_, err = q().Upsert([]map[string]any{
		{"code": "A", "amount": 10},
	}, []string{"code"}, nil)
	require.NoError(t, err)

	rows, err := q().OrderBy("code").Get()
	require.NoError(t, err)
	require.Len(t, rows, 2)
	assert.EqualValues(t, 10, rows[0]["amount"], "conflict updated in place")
}
