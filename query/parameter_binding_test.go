package query_test

import (
	"database/sql"
	"strings"
	"testing"

	"github.com/genesysflow/go-genesys/query"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

// bindingDB is a table holding one row whose values are themselves SQL.
func bindingDB(t *testing.T) *sql.DB {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.Exec(`CREATE TABLE users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		age INTEGER NOT NULL DEFAULT 0
	)`)
	require.NoError(t, err)

	return db
}

// The payloads a value could carry if it were ever concatenated into the
// statement rather than bound.
var injectionPayloads = []string{
	`' OR '1'='1`,
	`'; DROP TABLE users; --`,
	`" OR ""="`,
	`\'; DROP TABLE users; --`,
	`Robert'); DROP TABLE users; --`,
	`1; DELETE FROM users`,
	`' UNION SELECT id, name, age FROM users --`,
}

// A value never reaches the statement: it is sent separately, so the
// database treats it as data whatever it spells. This is the guarantee
// the whole query layer rests on.
func TestValuesAreBoundNotInterpolated(t *testing.T) {
	for _, payload := range injectionPayloads {
		sqlStr, bindings := query.New("sqlite", nil).Table("users").
			Where("name", payload).
			ToSQL()

		assert.NotContains(t, sqlStr, payload,
			"the value must not appear in the statement")
		assert.Equal(t, []any{payload}, bindings,
			"it must be sent as a binding instead")
		assert.Contains(t, sqlStr, "?")
	}
}

// Every clause that takes a value binds it - not just the common one.
func TestEveryValueClauseBinds(t *testing.T) {
	payload := `'; DROP TABLE users; --`

	cases := map[string]*query.Builder{
		"where":        query.New("sqlite", nil).Table("users").Where("name", payload),
		"orWhere":      query.New("sqlite", nil).Table("users").Where("id", 1).OrWhere("name", payload),
		"whereIn":      query.New("sqlite", nil).Table("users").WhereIn("name", payload, "other"),
		"whereNotIn":   query.New("sqlite", nil).Table("users").WhereNotIn("name", payload),
		"whereBetween": query.New("sqlite", nil).Table("users").WhereBetween("name", payload, payload),
		"whereLike":    query.New("sqlite", nil).Table("users").Where("name", "like", payload),
		"having":       query.New("sqlite", nil).Table("users").GroupBy("name").Having("name", "=", payload),
		"nested": query.New("sqlite", nil).Table("users").WhereGroup(func(b *query.Builder) {
			b.Where("name", payload)
		}),
	}

	for name, builder := range cases {
		sqlStr, bindings := builder.ToSQL()

		assert.NotContains(t, sqlStr, payload, "%s put the value in the statement", name)
		assert.Contains(t, bindings, any(payload), "%s did not bind the value", name)
	}
}

// Proof at the driver, not just in the compiled string: a row whose
// values are SQL survives a round trip unchanged, and the table it tried
// to drop is still there.
func TestInjectionPayloadsRoundTripAsData(t *testing.T) {
	db := bindingDB(t)

	for _, payload := range injectionPayloads {
		require.NoError(t, query.New("sqlite", db).Table("users").
			Insert(map[string]any{"name": payload, "age": 30}))
	}

	for _, payload := range injectionPayloads {
		row, err := query.New("sqlite", db).Table("users").
			Where("name", payload).
			First()
		require.NoError(t, err)
		require.NotNil(t, row, "the row for %q should be found by its exact value", payload)
		assert.Equal(t, payload, row["name"], "the value came back changed")
	}

	// Nothing was dropped, deleted or unioned in.
	count, err := query.New("sqlite", db).Table("users").Count()
	require.NoError(t, err)
	assert.Equal(t, int64(len(injectionPayloads)), count)
}

// The same through a write: an UPDATE whose value is SQL updates one
// row's text rather than running.
func TestUpdatesBindTheirValues(t *testing.T) {
	db := bindingDB(t)

	require.NoError(t, query.New("sqlite", db).Table("users").
		Insert(map[string]any{"name": "Ada", "age": 36}))

	payload := `'; DELETE FROM users; --`
	affected, err := query.New("sqlite", db).Table("users").
		Where("name", "Ada").
		Update(map[string]any{"name": payload})
	require.NoError(t, err)
	assert.Equal(t, int64(1), affected)

	count, err := query.New("sqlite", db).Table("users").Count()
	require.NoError(t, err)
	assert.Equal(t, int64(1), count, "the row must still be there")

	row, err := query.New("sqlite", db).Table("users").Where("name", payload).First()
	require.NoError(t, err)
	require.NotNil(t, row)
}

// What binding does and does not buy, stated precisely.
//
// It buys everything for values: a bound value is transmitted apart from
// the statement and can never add a clause, close a quote or start a
// second statement, whatever it spells.
//
// It buys nothing for the statement itself. This driver executes a
// trailing statement even when the call has bindings, so a mistake in
// the SQL - an unquoted identifier, a raw fragment built from input - is
// not "reads the wrong column" but "runs anything". That is why
// identifiers are quoted rather than trusted, and why SelectRaw and
// WhereRaw are documented as SQL rather than data.
func TestBindingProtectsValuesNotTheStatement(t *testing.T) {
	db := bindingDB(t)

	// A value that spells a second statement is stored, not run.
	require.NoError(t, query.New("sqlite", db).Table("users").
		Insert(map[string]any{"name": `x'); DROP TABLE users; --`, "age": 1}))

	var table string
	require.NoError(t,
		db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='users'").Scan(&table),
		"a bound value must not be able to drop the table")
	assert.Equal(t, "users", table)

	// The statement is a different matter: this driver runs what follows
	// a semicolon, bindings or not. Recorded so a driver change is
	// visible rather than silent.
	_, err := db.Exec("INSERT INTO users (name) VALUES (?); DROP TABLE users", "ada")
	require.NoError(t, err)

	err = db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='users'").Scan(&table)
	assert.ErrorIs(t, err, sql.ErrNoRows,
		"this driver executes trailing statements; if that changes, the note in docs/security.md should too")
}

// A raw fragment still binds its values: the SQL is the developer's, the
// values are not.
func TestWhereRawStillBindsItsValues(t *testing.T) {
	db := bindingDB(t)

	require.NoError(t, query.New("sqlite", db).Table("users").
		Insert(map[string]any{"name": "Ada", "age": 36}))

	payload := `' OR '1'='1`
	rows, err := query.New("sqlite", db).Table("users").
		WhereRaw("name = ?", payload).
		Get()
	require.NoError(t, err)
	assert.Empty(t, rows, "the payload is a value, so it matches nothing")

	sqlStr, bindings := query.New("sqlite", nil).Table("users").
		WhereRaw("name = ?", payload).
		ToSQL()
	assert.NotContains(t, sqlStr, payload)
	assert.Equal(t, []any{any(payload)}, bindings)
	assert.NotContains(t, strings.ToUpper(sqlStr), "OR '1'='1")
}
