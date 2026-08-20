package query_test

import (
	"testing"

	"github.com/genesysflow/go-genesys/query"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCompileMoreWhereVariants(t *testing.T) {
	sqlStr, bindings := query.New("sqlite", nil).Table("users").
		WhereBetween("age", 18, 65).
		WhereColumn("updated_at", ">", "created_at").
		WhereNotIn("status", "banned", "ghost").
		OrWhereIn("role", "admin").
		WhereNotNull("email").
		ToSQL()

	assert.Equal(t,
		`SELECT * FROM "users" WHERE "age" BETWEEN ? AND ? AND "updated_at" > "created_at" AND "status" NOT IN (?, ?) OR "role" IN (?) AND "email" IS NOT NULL`,
		sqlStr)
	assert.Equal(t, []any{18, 65, "banned", "ghost", "admin"}, bindings)
}

func TestCompileNilValueBecomesNullCheck(t *testing.T) {
	sqlStr, bindings := query.New("sqlite", nil).Table("users").
		Where("deleted_at", nil).
		Where("merged_at", "!=", nil).
		ToSQL()
	assert.Equal(t, `SELECT * FROM "users" WHERE "deleted_at" IS NULL AND "merged_at" IS NOT NULL`, sqlStr)
	assert.Empty(t, bindings)
}

func TestEmptyWhereNotInMatchesEverything(t *testing.T) {
	sqlStr, _ := query.New("sqlite", nil).Table("users").WhereNotIn("id").ToSQL()
	assert.Equal(t, `SELECT * FROM "users" WHERE 1 = 1`, sqlStr)
}

func TestWhereInAcceptsSlice(t *testing.T) {
	sqlStr, bindings := query.New("sqlite", nil).Table("users").
		WhereIn("id", []any{1, 2, 3}).
		ToSQL()
	assert.Equal(t, `SELECT * FROM "users" WHERE "id" IN (?, ?, ?)`, sqlStr)
	assert.Equal(t, []any{1, 2, 3}, bindings)
}

func TestCompileSelectModifiers(t *testing.T) {
	sqlStr, _ := query.New("sqlite", nil).Table("users").
		Select("id").
		AddSelect("name").
		Distinct().
		OrderByRaw("RANDOM()").
		Take(5).
		Skip(10).
		ToSQL()
	assert.Equal(t, `SELECT DISTINCT "id", "name" FROM "users" ORDER BY RANDOM() LIMIT 5 OFFSET 10`, sqlStr)
}

func TestLeftRightJoinAndHavingRaw(t *testing.T) {
	sqlStr, bindings := query.New("sqlite", nil).Table("users").
		LeftJoin("posts", "users.id", "=", "posts.user_id").
		RightJoin("teams", "users.team_id", "=", "teams.id").
		GroupBy("users.id").
		HavingRaw("COUNT(posts.id) > ?", 3).
		ToSQL()
	assert.Contains(t, sqlStr, "LEFT JOIN")
	assert.Contains(t, sqlStr, "RIGHT JOIN")
	assert.Contains(t, sqlStr, "HAVING COUNT(posts.id) > ?")
	assert.Equal(t, []any{3}, bindings)
}

func TestWhenConditionally(t *testing.T) {
	build := func(active bool) string {
		sqlStr, _ := query.New("sqlite", nil).Table("users").
			When(active, func(q *query.Builder) { q.Where("active", true) }).
			ToSQL()
		return sqlStr
	}
	assert.Contains(t, build(true), "WHERE")
	assert.NotContains(t, build(false), "WHERE")
}

func TestLatestAndOldest(t *testing.T) {
	sqlStr, _ := query.New("sqlite", nil).Table("users").Latest().ToSQL()
	assert.Contains(t, sqlStr, `ORDER BY "created_at" DESC`)

	sqlStr, _ = query.New("sqlite", nil).Table("users").Oldest("published_at").ToSQL()
	assert.Contains(t, sqlStr, `ORDER BY "published_at" ASC`)
}

func TestIntegrationValueMinMaxSum(t *testing.T) {
	exec := newTestDB(t)
	seedUsers(t, exec)
	table := func() *query.Builder { return query.New("sqlite", exec).Table("users") }

	name, err := table().Where("age", 41).Value("name")
	require.NoError(t, err)
	assert.Equal(t, "Carol", name)

	// Value with a dotted/aliased column resolves the right key.
	aliased, err := table().Where("age", 41).Value("users.name")
	require.NoError(t, err)
	assert.Equal(t, "Carol", aliased)

	minAge, err := table().Min("age")
	require.NoError(t, err)
	assert.EqualValues(t, 25, minAge)

	maxAge, err := table().Max("age")
	require.NoError(t, err)
	assert.EqualValues(t, 41, maxAge)

	total, err := table().Sum("age")
	require.NoError(t, err)
	assert.EqualValues(t, 96, total)

	count, err := table().Count("email")
	require.NoError(t, err)
	assert.EqualValues(t, 3, count)
}

func TestIntegrationDecrementAndBetween(t *testing.T) {
	exec := newTestDB(t)
	seedUsers(t, exec)
	table := func() *query.Builder { return query.New("sqlite", exec).Table("users") }

	_, err := table().Where("name", "Alice").Decrement("age", 5)
	require.NoError(t, err)
	age, err := table().Where("name", "Alice").Value("age")
	require.NoError(t, err)
	assert.EqualValues(t, 25, age)

	rows, err := table().WhereBetween("age", 24, 26).OrderBy("name").Get()
	require.NoError(t, err)
	assert.Len(t, rows, 2) // Alice (25) and Bob (25)
}

func TestInsertNothingIsNoop(t *testing.T) {
	exec := newTestDB(t)
	require.NoError(t, query.New("sqlite", exec).Table("users").Insert())
}
