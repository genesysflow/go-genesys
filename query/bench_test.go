package query_test

import (
	"database/sql"
	"fmt"
	"testing"

	"github.com/genesysflow/go-genesys/query"
)

func BenchmarkCompileSimpleSelect(b *testing.B) {
	for i := 0; i < b.N; i++ {
		query.New("sqlite", nil).Table("users").
			Where("active", true).
			OrderBy("name").
			Limit(10).
			ToSQL()
	}
}

func BenchmarkCompileComplexSelect(b *testing.B) {
	for i := 0; i < b.N; i++ {
		query.New("pgsql", nil).Table("users").
			Select("users.id", "users.name").
			Join("posts", "users.id", "=", "posts.user_id").
			Where("users.active", true).
			WhereIn("users.role", "admin", "editor", "author").
			WhereBetween("users.age", 18, 65).
			GroupBy("users.id").
			HavingRaw("COUNT(posts.id) > ?", 3).
			OrderByDesc("users.created_at").
			Limit(25).Offset(50).
			ToSQL()
	}
}

func BenchmarkCompileWhereExists(b *testing.B) {
	for i := 0; i < b.N; i++ {
		sub := query.New("pgsql", nil).Table("posts").
			WhereColumn("posts.user_id", "=", "users.id").
			Where("published", true)
		query.New("pgsql", nil).Table("users").
			Where("active", true).
			WhereExists(sub).
			ToSQL()
	}
}

func BenchmarkQueryGet(b *testing.B) {
	exec := newBenchDB(b, 100)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rows, err := query.New("sqlite", exec).Table("users").
			Where("age", ">", 25).Get()
		if err != nil {
			b.Fatal(err)
		}
		_ = rows
	}
}

// newBenchDB builds an in-memory table with n rows for benchmarks.
func newBenchDB(b *testing.B, n int) *sqliteExecutor {
	b.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { db.Close() })

	if _, err := db.Exec(`CREATE TABLE users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL, email TEXT NOT NULL,
		age INTEGER NOT NULL, active INTEGER NOT NULL DEFAULT 1)`); err != nil {
		b.Fatal(err)
	}
	exec := &sqliteExecutor{db: db}
	for i := 0; i < n; i++ {
		if err := query.New("sqlite", exec).Table("users").Insert(map[string]any{
			"name": fmt.Sprintf("User %d", i), "email": fmt.Sprintf("u%d@x.io", i),
			"age": 20 + i%40, "active": i % 2,
		}); err != nil {
			b.Fatal(err)
		}
	}
	return exec
}
