package database_test

import (
	"fmt"
	"testing"

	"github.com/genesysflow/go-genesys/database"
	_ "modernc.org/sqlite"
)

// benchSetup builds the blog schema with authors, posts, and tags.
func benchSetup(b *testing.B, authors, postsPer int) {
	b.Helper()
	manager := database.NewManager(database.Config{
		Default: "bench",
		Connections: map[string]database.ConnectionConfig{
			"bench": {Driver: "sqlite", Database: ":memory:"},
		},
	})
	b.Cleanup(func() {
		database.SetDefault(nil)
		manager.Close()
	})
	database.SetDefault(manager)

	for _, ddl := range []string{
		`CREATE TABLE authors (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT,
			created_at TIMESTAMP, updated_at TIMESTAMP)`,
		`CREATE TABLE profiles (id INTEGER PRIMARY KEY AUTOINCREMENT, author_id INTEGER, bio TEXT,
			created_at TIMESTAMP, updated_at TIMESTAMP)`,
		`CREATE TABLE articles (id INTEGER PRIMARY KEY AUTOINCREMENT, author_id INTEGER, title TEXT,
			created_at TIMESTAMP, updated_at TIMESTAMP)`,
		`CREATE TABLE tags (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT,
			created_at TIMESTAMP, updated_at TIMESTAMP)`,
		`CREATE TABLE article_tag (article_id INTEGER, tag_id INTEGER)`,
	} {
		if _, err := manager.Statement(ddl); err != nil {
			b.Fatal(err)
		}
	}

	for i := 0; i < authors; i++ {
		author := &Author{Name: fmt.Sprintf("Author %d", i)}
		if err := database.Create(author); err != nil {
			b.Fatal(err)
		}
		for j := 0; j < postsPer; j++ {
			if err := database.CreateFor(author, "Posts",
				&Article{Title: fmt.Sprintf("Post %d-%d", i, j)}); err != nil {
				b.Fatal(err)
			}
		}
	}
}

func BenchmarkScanRows(b *testing.B) {
	benchSetup(b, 100, 0)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		authors, err := database.All[Author]()
		if err != nil {
			b.Fatal(err)
		}
		if len(authors) != 100 {
			b.Fatalf("expected 100 authors, got %d", len(authors))
		}
	}
}

func BenchmarkEagerLoadHasMany(b *testing.B) {
	benchSetup(b, 50, 5)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		authors, err := database.Query[Author]().With("Posts").Get()
		if err != nil {
			b.Fatal(err)
		}
		if len(authors[0].Posts) != 5 {
			b.Fatalf("expected 5 posts, got %d", len(authors[0].Posts))
		}
	}
}

func BenchmarkCreate(b *testing.B) {
	benchSetup(b, 0, 0)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := database.Create(&Author{Name: "Bench"}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDirtyCheck(b *testing.B) {
	benchSetup(b, 1, 0)
	author, err := database.FirstWhere[Author]("name", "Author 0")
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if database.IsDirty(author) {
			b.Fatal("clean model reported dirty")
		}
	}
}

func BenchmarkWhereHas(b *testing.B) {
	benchSetup(b, 50, 3)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		authors, err := database.Query[Author]().Has("Posts").Get()
		if err != nil {
			b.Fatal(err)
		}
		if len(authors) != 50 {
			b.Fatalf("expected 50, got %d", len(authors))
		}
	}
}
