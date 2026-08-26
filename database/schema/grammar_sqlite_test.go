package schema

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The SQLite grammar's create path is covered indirectly elsewhere; these tests
// pin the ALTER and index statements, which nothing else reaches.

func TestSQLiteGrammarIsTheFallbackDriver(t *testing.T) {
	// Anything unrecognised falls back to SQLite rather than failing, so a typo
	// in a driver name produces SQLite SQL rather than an error.
	for _, driver := range []string{"sqlite", "sqlite3", "", "unknown", "MySQL"} {
		assert.IsType(t, &SQLiteGrammar{}, NewGrammar(driver), "driver %q", driver)
	}
}

func TestSQLiteCompileTableExists(t *testing.T) {
	g := &SQLiteGrammar{}

	assert.Equal(t,
		"SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='users'",
		g.CompileTableExists("users"))
}

func TestSQLiteCompileColumnTypes(t *testing.T) {
	tests := []struct {
		name  string
		build func(*Blueprint)
		want  string
	}{
		{
			name:  "auto-incrementing id",
			build: func(bp *Blueprint) { bp.ID() },
			want:  `"id" INTEGER PRIMARY KEY AUTOINCREMENT`,
		},
		{
			// SQLite's rowid alias must be INTEGER, so bigint collapses to it.
			name:  "bigint collapses to INTEGER",
			build: func(bp *Blueprint) { bp.BigIncrements("id") },
			want:  `"id" INTEGER PRIMARY KEY AUTOINCREMENT`,
		},
		{
			name:  "plain bigint is also INTEGER",
			build: func(bp *Blueprint) { bp.BigInteger("views") },
			want:  `"views" INTEGER NOT NULL`,
		},
		{
			name:  "varchar",
			build: func(bp *Blueprint) { bp.String("name", 100) },
			want:  `"name" VARCHAR(100) NOT NULL`,
		},
		{
			name:  "decimal",
			build: func(bp *Blueprint) { bp.Decimal("balance", 8, 2) },
			want:  `"balance" DECIMAL(8,2) NOT NULL`,
		},
		{
			name:  "float",
			build: func(bp *Blueprint) { bp.Float("rating").Nullable() },
			want:  `"rating" FLOAT`,
		},
		{
			name:  "datetime is kept as-is",
			build: func(bp *Blueprint) { bp.DateTime("seen_at").Nullable() },
			want:  `"seen_at" DATETIME`,
		},
		{
			// SQLite has no boolean literal, so a bool default renders as 1/0.
			name:  "true default renders as 1",
			build: func(bp *Blueprint) { bp.Boolean("active").Default(true) },
			want:  `"active" BOOLEAN NOT NULL DEFAULT 1`,
		},
		{
			name:  "false default renders as 0",
			build: func(bp *Blueprint) { bp.Boolean("banned").Default(false) },
			want:  `"banned" BOOLEAN NOT NULL DEFAULT 0`,
		},
		{
			// UNSIGNED is silently dropped; SQLite has no such modifier.
			name:  "foreign id drops the unsigned flag",
			build: func(bp *Blueprint) { bp.ForeignID("team_id") },
			want:  `"team_id" INTEGER NOT NULL`,
		},
		{
			name:  "column-level unique",
			build: func(bp *Blueprint) { bp.String("email", 255).Unique() },
			want:  `"email" VARCHAR(255) NOT NULL UNIQUE`,
		},
		{
			name:  "string default is quoted",
			build: func(bp *Blueprint) { bp.String("role", 20).Default("guest") },
			want:  `"role" VARCHAR(20) NOT NULL DEFAULT 'guest'`,
		},
	}

	g := &SQLiteGrammar{}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			bp := NewBlueprint("t")
			tc.build(bp)
			assert.Equal(t, "CREATE TABLE \"t\" (\n  "+tc.want+"\n)", g.CompileCreate(bp))
		})
	}
}

func TestSQLiteCompileCreateIndexes(t *testing.T) {
	g := &SQLiteGrammar{}

	bp := NewBlueprint("users")
	bp.String("slug", 50).Index()
	bp.String("name", 100)
	bp.Integer("age")
	bp.Index("name", "age")

	assert.Equal(t, []string{
		`CREATE INDEX "users_slug_index" ON "users" ("slug")`,
		`CREATE INDEX "users_name_age_index" ON "users" ("name", "age")`,
	}, g.CompileCreateIndexes(bp))

	assert.Empty(t, g.CompileCreateIndexes(NewBlueprint("users")))
}

func TestSQLiteCompileAddColumn(t *testing.T) {
	g := &SQLiteGrammar{}

	assert.Equal(t,
		`ALTER TABLE "posts" ADD COLUMN "title" VARCHAR(200) NOT NULL`,
		g.CompileAddColumn("posts", ColumnDefinition{Name: "title", Type: "varchar", Length: 200}))

	tests := []struct {
		name  string
		build func(*Blueprint)
		want  string
	}{
		{
			name:  "AddInteger with default",
			build: func(bp *Blueprint) { bp.AddInteger("hits").Default(0) },
			want:  `ALTER TABLE "posts" ADD COLUMN "hits" INTEGER NOT NULL DEFAULT 0`,
		},
		{
			name:  "AddBigInteger nullable",
			build: func(bp *Blueprint) { bp.AddBigInteger("size").Nullable() },
			want:  `ALTER TABLE "posts" ADD COLUMN "size" INTEGER`,
		},
		{
			name:  "AddText",
			build: func(bp *Blueprint) { bp.AddText("body") },
			want:  `ALTER TABLE "posts" ADD COLUMN "body" TEXT NOT NULL`,
		},
		{
			name:  "AddDecimal",
			build: func(bp *Blueprint) { bp.AddDecimal("price", 10, 2).Nullable() },
			want:  `ALTER TABLE "posts" ADD COLUMN "price" DECIMAL(10,2)`,
		},
		{
			name:  "AddFloat with default",
			build: func(bp *Blueprint) { bp.AddFloat("score").Default(1.5) },
			want:  `ALTER TABLE "posts" ADD COLUMN "score" FLOAT NOT NULL DEFAULT 1.5`,
		},
		{
			name:  "AddDateTime nullable",
			build: func(bp *Blueprint) { bp.AddDateTime("published_at").Nullable() },
			want:  `ALTER TABLE "posts" ADD COLUMN "published_at" DATETIME`,
		},
		{
			name:  "AddColumn with a raw type",
			build: func(bp *Blueprint) { bp.AddColumn("payload", "blob").Nullable() },
			want:  `ALTER TABLE "posts" ADD COLUMN "payload" BLOB`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			bp := NewBlueprint("posts")
			tc.build(bp)

			stmts, err := g.CompileAlter(bp)
			require.NoError(t, err)
			assert.Equal(t, []string{tc.want}, stmts)
		})
	}
}

func TestSQLiteCompileDropColumn(t *testing.T) {
	g := &SQLiteGrammar{}

	assert.Equal(t, `ALTER TABLE "posts" DROP COLUMN "body"`, g.CompileDropColumn("posts", "body"))

	// SQLite has no multi-column DROP, so each column gets its own statement.
	bp := NewBlueprint("posts")
	bp.DropColumn("old_a", "old_b")

	stmts, err := g.CompileAlter(bp)
	require.NoError(t, err)
	assert.Equal(t, []string{
		`ALTER TABLE "posts" DROP COLUMN "old_a"`,
		`ALTER TABLE "posts" DROP COLUMN "old_b"`,
	}, stmts)
}

func TestSQLiteCompileRenameColumn(t *testing.T) {
	g := &SQLiteGrammar{}

	assert.Equal(t,
		`ALTER TABLE "posts" RENAME COLUMN "headline" TO "title"`,
		g.CompileRenameColumn("posts", "headline", "title"))

	bp := NewBlueprint("posts")
	bp.RenameColumn("headline", "title")

	stmts, err := g.CompileAlter(bp)
	require.NoError(t, err)
	assert.Equal(t, []string{`ALTER TABLE "posts" RENAME COLUMN "headline" TO "title"`}, stmts)
}

func TestSQLiteCompileDropIndex(t *testing.T) {
	g := &SQLiteGrammar{}

	assert.Equal(t, `DROP INDEX IF EXISTS "posts_author_id_index"`,
		g.CompileDropIndex("posts", []string{"author_id"}))
	assert.Equal(t, `DROP INDEX IF EXISTS "posts_author_id_status_index"`,
		g.CompileDropIndex("posts", []string{"author_id", "status"}))

	bp := NewBlueprint("posts")
	bp.DropIndex("author_id")

	stmts, err := g.CompileAlter(bp)
	require.NoError(t, err)
	assert.Equal(t, []string{`DROP INDEX IF EXISTS "posts_author_id_index"`}, stmts)
}

func TestSQLiteCompileAlterWithNoCommands(t *testing.T) {
	stmts, err := (&SQLiteGrammar{}).CompileAlter(NewBlueprint("posts"))
	require.NoError(t, err)
	assert.Empty(t, stmts)
}
