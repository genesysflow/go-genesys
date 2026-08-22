package query_test

import (
	"testing"

	"github.com/genesysflow/go-genesys/query"
	"github.com/stretchr/testify/assert"
)

// The operator position is interpolated into SQL, so anything outside
// the whitelist must be rejected loudly rather than compiled.
func TestIllegalOperatorPanics(t *testing.T) {
	assert.PanicsWithValue(t,
		`query: illegal operator "= 18 OR 1=1 --"`,
		func() {
			query.New("sqlite", nil).Table("users").Where("age", "= 18 OR 1=1 --", 18)
		})

	assert.NotPanics(t, func() {
		query.New("sqlite", nil).Table("users").
			Where("age", ">=", 18).
			Where("name", "LIKE", "a%").
			WhereColumn("a", "!=", "b").
			Having("total", "<", 10)
	}, "whitelisted operators pass, case-insensitively")
}

// A column name is an identifier, not SQL. Select is the natural place
// for an application to pass a client-chosen field list, so a value that
// happens to contain parentheses must not become an expression.
func TestSelectDoesNotEmitClientSQL(t *testing.T) {
	sql, _ := query.New("sqlite", nil).Table("users").
		Select("id", "(SELECT password FROM users LIMIT 1)").
		ToSQL()

	// Quoted, so it is the (nonsense) identifier it was passed as
	// rather than a subquery the database will run.
	assert.Equal(t,
		`SELECT "id", "(SELECT password FROM users LIMIT 1)" FROM "users"`,
		sql)
}

// AddSelect is the same door.
func TestAddSelectDoesNotEmitClientSQL(t *testing.T) {
	sql, _ := query.New("sqlite", nil).Table("users").
		Select("id").
		AddSelect("(SELECT password FROM users)").
		ToSQL()

	assert.Equal(t,
		`SELECT "id", "(SELECT password FROM users)" FROM "users"`,
		sql)
}

// Expressions are still available, but only where the application asked
// for them by name.
func TestSelectRawEmitsTheExpression(t *testing.T) {
	sql, _ := query.New("sqlite", nil).Table("orders").
		Select("id").
		SelectRaw("COUNT(*) AS total").
		ToSQL()

	assert.Contains(t, sql, "COUNT(*) AS total")
	assert.Contains(t, sql, `"id"`)
}

// The aggregates keep working, since they build their own expressions.
func TestAggregateExpressionsStillCompile(t *testing.T) {
	sql, _ := query.New("sqlite", nil).Table("orders").
		SelectRaw("SUM(total) AS revenue").
		GroupBy("customer_id").
		ToSQL()

	assert.Contains(t, sql, "SUM(total) AS revenue")
}
