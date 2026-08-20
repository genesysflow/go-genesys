package query_test

import (
	"testing"

	"github.com/genesysflow/go-genesys/query"
	"github.com/stretchr/testify/assert"
)

func TestWhereExistsCompilation(t *testing.T) {
	sub := query.New("sqlite", nil).Table("posts").
		WhereColumn("posts.user_id", "=", "users.id").
		Where("published", true)

	sqlStr, bindings := query.New("sqlite", nil).Table("users").
		Where("active", true).
		WhereExists(sub).
		ToSQL()

	assert.Equal(t,
		`SELECT * FROM "users" WHERE "active" = ? AND EXISTS (SELECT * FROM "posts" WHERE "posts"."user_id" = "users"."id" AND "published" = ?)`,
		sqlStr)
	assert.Equal(t, []any{true, true}, bindings)
}

func TestWhereExistsRenumbersForPostgres(t *testing.T) {
	sub := query.New("pgsql", nil).Table("posts").
		WhereColumn("posts.user_id", "=", "users.id").
		Where("published", true).
		Where("views", ">", 10)

	sqlStr, bindings := query.New("pgsql", nil).Table("users").
		Where("active", true).
		WhereExists(sub).
		Where("age", ">=", 18).
		ToSQL()

	// The subquery's placeholders continue the outer numbering, and the
	// clause after it keeps counting.
	assert.Contains(t, sqlStr, `"active" = $1`)
	assert.Contains(t, sqlStr, `"published" = $2`)
	assert.Contains(t, sqlStr, `"views" > $3`)
	assert.Contains(t, sqlStr, `"age" >= $4`)
	assert.Equal(t, []any{true, true, 10, 18}, bindings)
}

func TestWhereNotExistsAndOrWhereExists(t *testing.T) {
	sub := func() *query.Builder {
		return query.New("sqlite", nil).Table("posts").
			WhereColumn("posts.user_id", "=", "users.id")
	}

	sqlStr, _ := query.New("sqlite", nil).Table("users").WhereNotExists(sub()).ToSQL()
	assert.Contains(t, sqlStr, "NOT EXISTS (")

	sqlStr, _ = query.New("sqlite", nil).Table("users").
		Where("admin", true).OrWhereExists(sub()).ToSQL()
	assert.Contains(t, sqlStr, "OR EXISTS (")
}

func TestWhereInSub(t *testing.T) {
	sub := query.New("sqlite", nil).Table("posts").Select("user_id").Where("published", true)

	sqlStr, bindings := query.New("sqlite", nil).Table("users").
		WhereInSub("id", sub).
		ToSQL()
	assert.Equal(t,
		`SELECT * FROM "users" WHERE "id" IN (SELECT "user_id" FROM "posts" WHERE "published" = ?)`,
		sqlStr)
	assert.Equal(t, []any{true}, bindings)
}
