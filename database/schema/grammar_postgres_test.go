package schema

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The Postgres grammar is exercised end-to-end elsewhere against a live
// container, but that only proves the SQL is accepted. These tests pin the
// exact text so a change in type mapping or clause order is visible without a
// database, and they reach the ALTER paths the container tests never take.

func TestPostgresGrammarSelectedByNewGrammar(t *testing.T) {
	for _, driver := range []string{"pgsql", "postgres", "postgresql"} {
		assert.IsType(t, &PostgresGrammar{}, NewGrammar(driver), "driver %q", driver)
	}
}

// The lookup is scoped to the schema the connection resolves unqualified names
// against, the way the MySQL grammar scopes it with table_schema = DATABASE().
// Unscoped, HasTable("users") would report true for an archive.users that the
// connection's search_path never reaches.
func TestPostgresCompileTableExists(t *testing.T) {
	g := &PostgresGrammar{}

	assert.Equal(t,
		"SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = current_schema() AND table_name = 'users'",
		g.CompileTableExists("users"))

	// The table name lands in a string literal, so its quotes must be doubled.
	assert.Equal(t,
		"SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = current_schema() AND table_name = 'us''ers'",
		g.CompileTableExists("us'ers"))
}

func TestPostgresCompileColumnTypes(t *testing.T) {
	tests := []struct {
		name  string
		build func(*Blueprint)
		want  string
	}{
		{
			name:  "auto-incrementing id becomes SERIAL",
			build: func(bp *Blueprint) { bp.ID() },
			want:  `"id" SERIAL PRIMARY KEY`,
		},
		{
			name:  "big auto-incrementing id becomes BIGSERIAL",
			build: func(bp *Blueprint) { bp.BigIncrements("id") },
			want:  `"id" BIGSERIAL PRIMARY KEY`,
		},
		{
			name:  "varchar carries its length",
			build: func(bp *Blueprint) { bp.String("name", 100) },
			want:  `"name" VARCHAR(100) NOT NULL`,
		},
		{
			name:  "text",
			build: func(bp *Blueprint) { bp.Text("bio") },
			want:  `"bio" TEXT NOT NULL`,
		},
		{
			name:  "integer",
			build: func(bp *Blueprint) { bp.Integer("age") },
			want:  `"age" INTEGER NOT NULL`,
		},
		{
			name:  "bigint",
			build: func(bp *Blueprint) { bp.BigInteger("views") },
			want:  `"views" BIGINT NOT NULL`,
		},
		{
			name:  "boolean",
			build: func(bp *Blueprint) { bp.Boolean("active") },
			want:  `"active" BOOLEAN NOT NULL`,
		},
		{
			name:  "decimal",
			build: func(bp *Blueprint) { bp.Decimal("balance", 8, 2) },
			want:  `"balance" DECIMAL(8,2) NOT NULL`,
		},
		{
			name:  "float",
			build: func(bp *Blueprint) { bp.Float("rating") },
			want:  `"rating" FLOAT NOT NULL`,
		},
		{
			// Postgres has no DATETIME; the grammar rewrites it to TIMESTAMP.
			name:  "datetime is rewritten to TIMESTAMP",
			build: func(bp *Blueprint) { bp.DateTime("seen_at") },
			want:  `"seen_at" TIMESTAMP NOT NULL`,
		},
		{
			name:  "timestamp",
			build: func(bp *Blueprint) { bp.Timestamp("created_at") },
			want:  `"created_at" TIMESTAMP NOT NULL`,
		},
		{
			name: "unknown types pass through upper-cased",
			build: func(bp *Blueprint) {
				bp.columns = append(bp.columns, ColumnDefinition{Name: "payload", Type: "jsonb"})
			},
			want: `"payload" JSONB NOT NULL`,
		},
		{
			name:  "nullable omits NOT NULL",
			build: func(bp *Blueprint) { bp.Text("bio").Nullable() },
			want:  `"bio" TEXT`,
		},
		{
			// Postgres has no UNSIGNED, so ForeignID's flag is dropped.
			name:  "foreign id is a plain bigint",
			build: func(bp *Blueprint) { bp.ForeignID("team_id") },
			want:  `"team_id" BIGINT NOT NULL`,
		},
		{
			name:  "column-level unique",
			build: func(bp *Blueprint) { bp.String("email", 255).Unique() },
			want:  `"email" VARCHAR(255) NOT NULL UNIQUE`,
		},
		{
			name:  "booleans render as true/false, not 1/0",
			build: func(bp *Blueprint) { bp.Boolean("active").Default(true) },
			want:  `"active" BOOLEAN NOT NULL DEFAULT true`,
		},
		{
			name:  "false default",
			build: func(bp *Blueprint) { bp.Boolean("banned").Default(false) },
			want:  `"banned" BOOLEAN NOT NULL DEFAULT false`,
		},
		{
			name:  "numeric default is unquoted",
			build: func(bp *Blueprint) { bp.Integer("hits").Default(0) },
			want:  `"hits" INTEGER NOT NULL DEFAULT 0`,
		},
	}

	g := &PostgresGrammar{}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			bp := NewBlueprint("t")
			tc.build(bp)
			assert.Equal(t, "CREATE TABLE \"t\" (\n  "+tc.want+"\n)", g.CompileCreate(bp))
		})
	}
}

func TestPostgresCompileCreateFullTable(t *testing.T) {
	bp := NewBlueprint("users")
	bp.ID()
	bp.String("name", 100)
	bp.String("email", 255).Unique()
	bp.Boolean("active").Default(true)
	bp.ForeignID("team_id")
	bp.Timestamps()
	bp.Unique("team_id", "name")

	want := "CREATE TABLE \"users\" (\n" +
		"  \"id\" SERIAL PRIMARY KEY,\n" +
		"  \"name\" VARCHAR(100) NOT NULL,\n" +
		"  \"email\" VARCHAR(255) NOT NULL UNIQUE,\n" +
		"  \"active\" BOOLEAN NOT NULL DEFAULT true,\n" +
		"  \"team_id\" BIGINT NOT NULL,\n" +
		"  \"created_at\" TIMESTAMP,\n" +
		"  \"updated_at\" TIMESTAMP,\n" +
		"  CONSTRAINT \"users_team_id_name_unique\" UNIQUE (\"team_id\", \"name\")\n" +
		")"

	assert.Equal(t, want, (&PostgresGrammar{}).CompileCreate(bp))
}

func TestPostgresCompileCreateWithTableLevelPrimary(t *testing.T) {
	bp := NewBlueprint("role_user")
	bp.BigInteger("role_id")
	bp.BigInteger("user_id")
	bp.Primary("role_id", "user_id")

	want := "CREATE TABLE \"role_user\" (\n" +
		"  \"role_id\" BIGINT NOT NULL,\n" +
		"  \"user_id\" BIGINT NOT NULL,\n" +
		"  PRIMARY KEY (\"role_id\", \"user_id\")\n" +
		")"

	assert.Equal(t, want, (&PostgresGrammar{}).CompileCreate(bp))
}

func TestPostgresCompileCreateIndexes(t *testing.T) {
	g := &PostgresGrammar{}

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

func TestPostgresCompileAddColumn(t *testing.T) {
	g := &PostgresGrammar{}

	tests := []struct {
		name  string
		build func(*Blueprint)
		want  string
	}{
		{
			name:  "AddColumn with a raw type",
			build: func(bp *Blueprint) { bp.AddColumn("payload", "jsonb") },
			want:  `ALTER TABLE "posts" ADD COLUMN "payload" JSONB NOT NULL`,
		},
		{
			name:  "AddString",
			build: func(bp *Blueprint) { bp.AddString("slug", 64) },
			want:  `ALTER TABLE "posts" ADD COLUMN "slug" VARCHAR(64) NOT NULL`,
		},
		{
			name:  "AddInteger with default",
			build: func(bp *Blueprint) { bp.AddInteger("hits").Default(0) },
			want:  `ALTER TABLE "posts" ADD COLUMN "hits" INTEGER NOT NULL DEFAULT 0`,
		},
		{
			name:  "AddBigInteger nullable",
			build: func(bp *Blueprint) { bp.AddBigInteger("size").Nullable() },
			want:  `ALTER TABLE "posts" ADD COLUMN "size" BIGINT`,
		},
		{
			name:  "AddText",
			build: func(bp *Blueprint) { bp.AddText("body") },
			want:  `ALTER TABLE "posts" ADD COLUMN "body" TEXT NOT NULL`,
		},
		{
			name:  "AddBoolean with false default",
			build: func(bp *Blueprint) { bp.AddBoolean("draft").Default(false) },
			want:  `ALTER TABLE "posts" ADD COLUMN "draft" BOOLEAN NOT NULL DEFAULT false`,
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
			name:  "AddDateTime becomes TIMESTAMP",
			build: func(bp *Blueprint) { bp.AddDateTime("published_at").Nullable() },
			want:  `ALTER TABLE "posts" ADD COLUMN "published_at" TIMESTAMP`,
		},
		{
			name:  "AddTimestamp",
			build: func(bp *Blueprint) { bp.AddTimestamp("edited_at").Nullable() },
			want:  `ALTER TABLE "posts" ADD COLUMN "edited_at" TIMESTAMP`,
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

// An auto-incrementing column added later still becomes a serial type; the
// NOT NULL is suppressed because the sequence supplies the value.
func TestPostgresCompileAddColumnAutoIncrement(t *testing.T) {
	g := &PostgresGrammar{}

	assert.Equal(t,
		`ALTER TABLE "posts" ADD COLUMN "n" BIGSERIAL`,
		g.CompileAddColumn("posts", ColumnDefinition{Name: "n", Type: "bigint", AutoIncrement: true}))

	assert.Equal(t,
		`ALTER TABLE "posts" ADD COLUMN "n" SERIAL`,
		g.CompileAddColumn("posts", ColumnDefinition{Name: "n", Type: "integer", AutoIncrement: true}))
}

func TestPostgresCompileDropColumn(t *testing.T) {
	g := &PostgresGrammar{}

	assert.Equal(t, `ALTER TABLE "posts" DROP COLUMN "body"`, g.CompileDropColumn("posts", "body"))

	bp := NewBlueprint("posts")
	bp.DropColumn("old_a", "old_b")

	stmts, err := g.CompileAlter(bp)
	require.NoError(t, err)
	assert.Equal(t, []string{
		`ALTER TABLE "posts" DROP COLUMN "old_a"`,
		`ALTER TABLE "posts" DROP COLUMN "old_b"`,
	}, stmts)
}

func TestPostgresCompileRenameColumn(t *testing.T) {
	g := &PostgresGrammar{}

	assert.Equal(t,
		`ALTER TABLE "posts" RENAME COLUMN "headline" TO "title"`,
		g.CompileRenameColumn("posts", "headline", "title"))
}

// Postgres splits a modification into one statement per aspect, so a caller can
// change nullability without restating the type — the opposite of MySQL.
func TestPostgresCompileModifyColumnTypeMapping(t *testing.T) {
	g := &PostgresGrammar{}

	tests := []struct {
		name string
		set  func(*ColumnDefinition)
		want []string
	}{
		{
			name: "text",
			set:  func(c *ColumnDefinition) { c.Text() },
			want: []string{`ALTER TABLE "posts" ALTER COLUMN "c" TYPE TEXT`},
		},
		{
			name: "integer",
			set:  func(c *ColumnDefinition) { c.Integer() },
			want: []string{`ALTER TABLE "posts" ALTER COLUMN "c" TYPE INTEGER`},
		},
		{
			name: "bigint",
			set:  func(c *ColumnDefinition) { c.BigInteger() },
			want: []string{`ALTER TABLE "posts" ALTER COLUMN "c" TYPE BIGINT`},
		},
		{
			name: "boolean",
			set:  func(c *ColumnDefinition) { c.Boolean() },
			want: []string{`ALTER TABLE "posts" ALTER COLUMN "c" TYPE BOOLEAN`},
		},
		{
			name: "decimal",
			set:  func(c *ColumnDefinition) { c.Decimal(10, 2) },
			want: []string{`ALTER TABLE "posts" ALTER COLUMN "c" TYPE DECIMAL(10,2)`},
		},
		{
			name: "float",
			set:  func(c *ColumnDefinition) { c.Float() },
			want: []string{`ALTER TABLE "posts" ALTER COLUMN "c" TYPE FLOAT`},
		},
		{
			name: "datetime is rewritten to TIMESTAMP",
			set:  func(c *ColumnDefinition) { c.DateTime() },
			want: []string{`ALTER TABLE "posts" ALTER COLUMN "c" TYPE TIMESTAMP`},
		},
		{
			name: "timestamp",
			set:  func(c *ColumnDefinition) { c.Timestamp() },
			want: []string{`ALTER TABLE "posts" ALTER COLUMN "c" TYPE TIMESTAMP`},
		},
		{
			name: "type, nullability and default in declaration order",
			set:  func(c *ColumnDefinition) { c.Boolean().Nullable().Default(true) },
			want: []string{
				`ALTER TABLE "posts" ALTER COLUMN "c" TYPE BOOLEAN`,
				`ALTER TABLE "posts" ALTER COLUMN "c" DROP NOT NULL`,
				`ALTER TABLE "posts" ALTER COLUMN "c" SET DEFAULT true`,
			},
		},
		{
			name: "numeric default",
			set:  func(c *ColumnDefinition) { c.Integer().Default(7) },
			want: []string{
				`ALTER TABLE "posts" ALTER COLUMN "c" TYPE INTEGER`,
				`ALTER TABLE "posts" ALTER COLUMN "c" SET DEFAULT 7`,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			col := ColumnDefinition{Name: "c"}
			tc.set(&col)

			stmts, err := g.CompileModifyColumn("posts", col)
			require.NoError(t, err)
			assert.Equal(t, tc.want, stmts)
		})
	}
}

// Nullable() and its absence are distinguishable because the blueprint records
// whether nullability was set at all, so a type-only modify leaves it alone.
func TestPostgresCompileModifyColumnSetsNotNullOnlyWhenAsked(t *testing.T) {
	g := &PostgresGrammar{}

	col := ColumnDefinition{Name: "c", NullableExplicitlySet: true, IsNullable: false}
	stmts, err := g.CompileModifyColumn("posts", col)
	require.NoError(t, err)
	assert.Equal(t, []string{`ALTER TABLE "posts" ALTER COLUMN "c" SET NOT NULL`}, stmts)

	// No type, no explicit nullability, no default: nothing to do, and the
	// grammar says so rather than emitting an empty batch.
	_, err = g.CompileModifyColumn("posts", ColumnDefinition{Name: "c"})
	require.Error(t, err)
}

func TestPostgresCompileDropIndex(t *testing.T) {
	g := &PostgresGrammar{}

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

// Postgres names an inline UNIQUE constraint "<table>_<cols>_key" while this
// package names the ones it creates "<table>_<cols>_unique", so a drop has to
// try both to work against tables it did not create.
func TestPostgresCompileDropUniqueTriesBothNamingConventions(t *testing.T) {
	g := &PostgresGrammar{}

	stmts, err := g.CompileDropUnique("users", []string{"email"})
	require.NoError(t, err)
	assert.Equal(t, []string{
		`ALTER TABLE "users" DROP CONSTRAINT IF EXISTS "users_email_unique"`,
		`ALTER TABLE "users" DROP CONSTRAINT IF EXISTS "users_email_key"`,
	}, stmts)

	stmts, err = g.CompileDropUnique("users", []string{"team_id", "email"})
	require.NoError(t, err)
	assert.Equal(t, []string{
		`ALTER TABLE "users" DROP CONSTRAINT IF EXISTS "users_team_id_email_unique"`,
		`ALTER TABLE "users" DROP CONSTRAINT IF EXISTS "users_team_id_email_key"`,
	}, stmts)
}

func TestPostgresCompileDropPrimary(t *testing.T) {
	g := &PostgresGrammar{}

	stmt, err := g.CompileDropPrimary("users")
	require.NoError(t, err)
	assert.Equal(t, `ALTER TABLE "users" DROP CONSTRAINT IF EXISTS "users_pkey"`, stmt)

	bp := NewBlueprint("users")
	bp.DropPrimary()

	stmts, err := g.CompileAlter(bp)
	require.NoError(t, err)
	assert.Equal(t, []string{`ALTER TABLE "users" DROP CONSTRAINT IF EXISTS "users_pkey"`}, stmts)
}

func TestPostgresCompileAlterPreservesCommandOrder(t *testing.T) {
	bp := NewBlueprint("posts")
	bp.AddString("slug", 64)
	bp.DropColumn("legacy_slug")
	bp.RenameColumn("headline", "title")
	bp.ModifyColumn("summary").String(500).Nullable()
	bp.DropIndex("author_id")
	bp.DropUnique("slug")
	bp.DropPrimary()

	stmts, err := (&PostgresGrammar{}).CompileAlter(bp)
	require.NoError(t, err)
	assert.Equal(t, []string{
		`ALTER TABLE "posts" ADD COLUMN "slug" VARCHAR(64) NOT NULL`,
		`ALTER TABLE "posts" DROP COLUMN "legacy_slug"`,
		`ALTER TABLE "posts" RENAME COLUMN "headline" TO "title"`,
		`ALTER TABLE "posts" ALTER COLUMN "summary" TYPE VARCHAR(500)`,
		`ALTER TABLE "posts" ALTER COLUMN "summary" DROP NOT NULL`,
		`DROP INDEX IF EXISTS "posts_author_id_index"`,
		`ALTER TABLE "posts" DROP CONSTRAINT IF EXISTS "posts_slug_unique"`,
		`ALTER TABLE "posts" DROP CONSTRAINT IF EXISTS "posts_slug_key"`,
		`ALTER TABLE "posts" DROP CONSTRAINT IF EXISTS "posts_pkey"`,
	}, stmts)
}

func TestPostgresCompileAlterWithNoCommands(t *testing.T) {
	stmts, err := (&PostgresGrammar{}).CompileAlter(NewBlueprint("posts"))
	require.NoError(t, err)
	assert.Empty(t, stmts)
}

// A modify that cannot compile aborts the whole batch: returning the statements
// compiled so far would let a caller apply half a migration.
func TestPostgresCompileAlterAbortsOnModifyError(t *testing.T) {
	bp := NewBlueprint("posts")
	bp.AddString("slug", 64)
	bp.ModifyColumn("summary") // requests no change

	stmts, err := (&PostgresGrammar{}).CompileAlter(bp)
	require.Error(t, err)
	assert.Nil(t, stmts)
}

// PostgreSQL cannot attach a comment to ADD COLUMN, so a commented column adds
// a second statement to the batch, immediately after the column exists.
func TestPostgresCompileAlterEmitsColumnComments(t *testing.T) {
	bp := NewBlueprint("posts")
	bp.AddString("slug", 64).Comment("url segment")
	bp.AddString("title", 200)

	stmts, err := (&PostgresGrammar{}).CompileAlter(bp)
	require.NoError(t, err)
	assert.Equal(t, []string{
		`ALTER TABLE "posts" ADD COLUMN "slug" VARCHAR(64) NOT NULL`,
		`COMMENT ON COLUMN "posts"."slug" IS 'url segment'`,
		`ALTER TABLE "posts" ADD COLUMN "title" VARCHAR(200) NOT NULL`,
	}, stmts)
}
