package schema

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The Add* helpers record commands rather than columns, which is what keeps a
// Table() migration distinguishable from a Create() one. These tests check the
// recorded shape directly; the grammar files check what it compiles to.

func TestBlueprintAddHelpersRecordCommandsNotColumns(t *testing.T) {
	tests := []struct {
		name          string
		build         func(*Blueprint) *ColumnDefinition
		wantType      string
		wantLength    int
		wantPrecision int
		wantScale     int
	}{
		{
			name:     "AddColumn takes the type verbatim",
			build:    func(bp *Blueprint) *ColumnDefinition { return bp.AddColumn("payload", "jsonb") },
			wantType: "jsonb",
		},
		{
			name:       "AddString defaults to 255",
			build:      func(bp *Blueprint) *ColumnDefinition { return bp.AddString("slug") },
			wantType:   "varchar",
			wantLength: 255,
		},
		{
			name:       "AddString honours an explicit length",
			build:      func(bp *Blueprint) *ColumnDefinition { return bp.AddString("slug", 64) },
			wantType:   "varchar",
			wantLength: 64,
		},
		{
			name:     "AddInteger",
			build:    func(bp *Blueprint) *ColumnDefinition { return bp.AddInteger("hits") },
			wantType: "integer",
		},
		{
			name:     "AddBigInteger",
			build:    func(bp *Blueprint) *ColumnDefinition { return bp.AddBigInteger("size") },
			wantType: "bigint",
		},
		{
			name:     "AddText",
			build:    func(bp *Blueprint) *ColumnDefinition { return bp.AddText("body") },
			wantType: "text",
		},
		{
			name:     "AddBoolean",
			build:    func(bp *Blueprint) *ColumnDefinition { return bp.AddBoolean("draft") },
			wantType: "boolean",
		},
		{
			name:     "AddTimestamp",
			build:    func(bp *Blueprint) *ColumnDefinition { return bp.AddTimestamp("edited_at") },
			wantType: "timestamp",
		},
		{
			name:          "AddDecimal",
			build:         func(bp *Blueprint) *ColumnDefinition { return bp.AddDecimal("price", 10, 2) },
			wantType:      "decimal",
			wantPrecision: 10,
			wantScale:     2,
		},
		{
			name:     "AddFloat",
			build:    func(bp *Blueprint) *ColumnDefinition { return bp.AddFloat("score") },
			wantType: "float",
		},
		{
			name:     "AddDateTime",
			build:    func(bp *Blueprint) *ColumnDefinition { return bp.AddDateTime("published_at") },
			wantType: "datetime",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			bp := NewBlueprint("posts")
			col := tc.build(bp)

			// Nothing lands in columns; Builder.Table rejects a blueprint that
			// populated those, so an Add* helper writing there would break it.
			assert.Empty(t, bp.columns)
			require.Len(t, bp.commands, 1)
			assert.Equal(t, "add", bp.commands[0].Type)

			require.NotNil(t, bp.commands[0].Column)
			assert.Equal(t, tc.wantType, bp.commands[0].Column.Type)
			assert.Equal(t, tc.wantLength, bp.commands[0].Column.Length)
			assert.Equal(t, tc.wantPrecision, bp.commands[0].Column.Precision)
			assert.Equal(t, tc.wantScale, bp.commands[0].Column.Scale)

			// The returned pointer must address the stored command, or fluent
			// modifiers would be applied to a discarded copy.
			require.Same(t, bp.commands[0].Column, col)
			col.Nullable()
			assert.True(t, bp.commands[0].Column.IsNullable)
		})
	}
}

func TestBlueprintDropIndexRecordsColumns(t *testing.T) {
	bp := NewBlueprint("posts")
	bp.DropIndex("author_id", "status")

	require.Len(t, bp.commands, 1)
	assert.Equal(t, "dropIndex", bp.commands[0].Type)
	assert.Equal(t, []string{"author_id", "status"}, bp.commands[0].Columns)
}

func TestBlueprintDropUniqueAndDropPrimaryRecordCommands(t *testing.T) {
	bp := NewBlueprint("users")
	bp.DropUnique("email")
	bp.DropPrimary()

	require.Len(t, bp.commands, 2)
	assert.Equal(t, "dropUnique", bp.commands[0].Type)
	assert.Equal(t, []string{"email"}, bp.commands[0].Columns)
	assert.Equal(t, "dropPrimary", bp.commands[1].Type)
	assert.Empty(t, bp.commands[1].Columns)
}

// The type setters exist so ModifyColumn can restate a definition; they mutate
// the receiver and return it for chaining.
func TestColumnDefinitionTypeSetters(t *testing.T) {
	tests := []struct {
		name          string
		set           func(*ColumnDefinition) *ColumnDefinition
		wantType      string
		wantLength    int
		wantPrecision int
		wantScale     int
	}{
		{
			name:       "String defaults to 255",
			set:        func(c *ColumnDefinition) *ColumnDefinition { return c.String() },
			wantType:   "varchar",
			wantLength: 255,
		},
		{
			name:       "String with an explicit length",
			set:        func(c *ColumnDefinition) *ColumnDefinition { return c.String(50) },
			wantType:   "varchar",
			wantLength: 50,
		},
		{
			name:     "Text",
			set:      func(c *ColumnDefinition) *ColumnDefinition { return c.Text() },
			wantType: "text",
		},
		{
			name:     "Integer",
			set:      func(c *ColumnDefinition) *ColumnDefinition { return c.Integer() },
			wantType: "integer",
		},
		{
			name:     "BigInteger",
			set:      func(c *ColumnDefinition) *ColumnDefinition { return c.BigInteger() },
			wantType: "bigint",
		},
		{
			name:     "Boolean",
			set:      func(c *ColumnDefinition) *ColumnDefinition { return c.Boolean() },
			wantType: "boolean",
		},
		{
			name:          "Decimal",
			set:           func(c *ColumnDefinition) *ColumnDefinition { return c.Decimal(12, 4) },
			wantType:      "decimal",
			wantPrecision: 12,
			wantScale:     4,
		},
		{
			name:     "Float",
			set:      func(c *ColumnDefinition) *ColumnDefinition { return c.Float() },
			wantType: "float",
		},
		{
			name:     "DateTime",
			set:      func(c *ColumnDefinition) *ColumnDefinition { return c.DateTime() },
			wantType: "datetime",
		},
		{
			name:     "Timestamp",
			set:      func(c *ColumnDefinition) *ColumnDefinition { return c.Timestamp() },
			wantType: "timestamp",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			col := &ColumnDefinition{Name: "c"}
			got := tc.set(col)

			require.Same(t, col, got, "setters must return the receiver for chaining")
			assert.Equal(t, tc.wantType, col.Type)
			assert.Equal(t, tc.wantLength, col.Length)
			assert.Equal(t, tc.wantPrecision, col.Precision)
			assert.Equal(t, tc.wantScale, col.Scale)
			assert.Equal(t, "c", col.Name, "a type setter must not disturb the name")
		})
	}
}

// ModifyColumn distinguishes "leave it alone" from "make it nullable"; without
// the explicit-set flags a zero value would read as an instruction.
func TestColumnDefinitionTracksExplicitlySetModifiers(t *testing.T) {
	plain := ColumnDefinition{Name: "c"}
	assert.False(t, plain.NullableExplicitlySet)
	assert.False(t, plain.DefaultExplicitlySet)

	nullable := ColumnDefinition{Name: "c"}
	nullable.Nullable()
	assert.True(t, nullable.NullableExplicitlySet)
	assert.True(t, nullable.IsNullable)

	// Default(nil) means "drop the default", which is why the flag is separate
	// from the value being non-nil.
	dropped := ColumnDefinition{Name: "c"}
	dropped.Default(nil)
	assert.True(t, dropped.DefaultExplicitlySet)
	assert.Nil(t, dropped.DefaultValue)
}

// A comment is emitted wherever the dialect has somewhere to put it: MySQL
// writes it inline in the column definition, PostgreSQL as a separate
// COMMENT ON COLUMN statement that Builder.Create runs once the table exists,
// and SQLite drops it because SQLite has no column comments at all.
func TestColumnCommentIsEmittedWhereTheDialectSupportsIt(t *testing.T) {
	bp := NewBlueprint("users")
	bp.String("email", 255).Comment("the login address")

	require.Equal(t, "the login address", bp.columns[0].ColumnComment)

	mysql := &MySQLGrammar{}
	assert.Equal(t,
		"CREATE TABLE `users` (\n  `email` VARCHAR(255) NOT NULL COMMENT 'the login address'\n)",
		mysql.CompileCreate(bp))
	assert.Empty(t, mysql.CompileComments(bp), "MySQL already wrote it inline")

	postgres := &PostgresGrammar{}
	assert.NotContains(t, postgres.CompileCreate(bp), "the login address",
		"PostgreSQL has no inline column comment")
	assert.Equal(t,
		[]string{`COMMENT ON COLUMN "users"."email" IS 'the login address'`},
		postgres.CompileComments(bp))

	sqlite := &SQLiteGrammar{}
	assert.NotContains(t, sqlite.CompileCreate(bp), "the login address")
	assert.Empty(t, sqlite.CompileComments(bp), "SQLite has no column comments")
}

// The comment reaches SQL as a string literal, so an embedded quote has to be
// doubled; unescaped it would close the literal and leave the rest to be parsed
// as SQL.
func TestColumnCommentEscapesQuotes(t *testing.T) {
	bp := NewBlueprint("users")
	bp.String("email", 255).Comment("the user's own address")

	assert.Contains(t, (&MySQLGrammar{}).CompileCreate(bp),
		"COMMENT 'the user''s own address'")
	assert.Equal(t,
		[]string{`COMMENT ON COLUMN "users"."email" IS 'the user''s own address'`},
		(&PostgresGrammar{}).CompileComments(bp))
}

// Columns without a comment contribute nothing, so an uncommented table runs no
// extra statements after its CREATE TABLE.
func TestCompileCommentsSkipsUncommentedColumns(t *testing.T) {
	bp := NewBlueprint("users")
	bp.ID()
	bp.String("email", 255)
	bp.String("name", 100).Comment("display name")

	assert.Equal(t,
		[]string{`COMMENT ON COLUMN "users"."name" IS 'display name'`},
		(&PostgresGrammar{}).CompileComments(bp))
	assert.Empty(t, (&PostgresGrammar{}).CompileComments(NewBlueprint("users")))
}

// Two columns flagged Primary are one composite key, and a table has exactly
// one primary key: the clause is emitted once, as a table constraint, and the
// inline PRIMARY KEY is left off the columns that feed it. Emitting both would
// declare three primary keys and every dialect rejects that.
func TestCompositeColumnPrimaryKeyIsDeclaredOnce(t *testing.T) {
	newBlueprint := func() *Blueprint {
		bp := NewBlueprint("role_user")
		bp.Integer("role_id").Primary = true
		bp.Integer("user_id").Primary = true
		return bp
	}

	assert.Equal(t,
		"CREATE TABLE \"role_user\" (\n"+
			"  \"role_id\" INTEGER,\n"+
			"  \"user_id\" INTEGER,\n"+
			"  PRIMARY KEY (\"role_id\", \"user_id\")\n)",
		(&SQLiteGrammar{}).CompileCreate(newBlueprint()))

	assert.Equal(t,
		"CREATE TABLE \"role_user\" (\n"+
			"  \"role_id\" INTEGER,\n"+
			"  \"user_id\" INTEGER,\n"+
			"  PRIMARY KEY (\"role_id\", \"user_id\")\n)",
		(&PostgresGrammar{}).CompileCreate(newBlueprint()))

	assert.Equal(t,
		"CREATE TABLE `role_user` (\n"+
			"  `role_id` INT,\n"+
			"  `user_id` INT,\n"+
			"  PRIMARY KEY (`role_id`, `user_id`)\n)",
		(&MySQLGrammar{}).CompileCreate(newBlueprint()))
}

// One flagged column is not composite: it keeps the inline form, because there
// is no table-level clause to move it to.
func TestSingleColumnPrimaryKeyStaysInline(t *testing.T) {
	newBlueprint := func() *Blueprint {
		bp := NewBlueprint("sessions")
		bp.String("id", 64).Primary = true
		bp.Text("payload")
		return bp
	}

	assert.Equal(t,
		"CREATE TABLE \"sessions\" (\n"+
			"  \"id\" VARCHAR(64) PRIMARY KEY,\n"+
			"  \"payload\" TEXT NOT NULL\n)",
		(&SQLiteGrammar{}).CompileCreate(newBlueprint()))

	assert.Equal(t,
		"CREATE TABLE \"sessions\" (\n"+
			"  \"id\" VARCHAR(64) PRIMARY KEY,\n"+
			"  \"payload\" TEXT NOT NULL\n)",
		(&PostgresGrammar{}).CompileCreate(newBlueprint()))

	assert.Equal(t,
		"CREATE TABLE `sessions` (\n"+
			"  `id` VARCHAR(64) PRIMARY KEY,\n"+
			"  `payload` TEXT NOT NULL\n)",
		(&MySQLGrammar{}).CompileCreate(newBlueprint()))
}

// An auto-incrementing key carries its PRIMARY KEY inline on every dialect, so
// it is not counted towards a composite key and nothing is suppressed.
func TestAutoIncrementPrimaryKeyIsNotTreatedAsComposite(t *testing.T) {
	bp := NewBlueprint("users")
	bp.ID()
	bp.String("email", 255)

	assert.Equal(t,
		"CREATE TABLE \"users\" (\n"+
			"  \"id\" SERIAL PRIMARY KEY,\n"+
			"  \"email\" VARCHAR(255) NOT NULL\n)",
		(&PostgresGrammar{}).CompileCreate(bp))
	assert.Empty(t, columnPrimaryKeys(bp, (&PostgresGrammar{}).WrapColumn))
}

// wrapAll is the shared helper behind every comma-separated identifier list.
func TestWrapAll(t *testing.T) {
	assert.Equal(t, []string{`"a"`, `"b"`},
		wrapAll([]string{"a", "b"}, (&PostgresGrammar{}).WrapColumn))
	assert.Equal(t, []string{"`a`", "`b`"},
		wrapAll([]string{"a", "b"}, (&MySQLGrammar{}).WrapColumn))
	assert.Empty(t, wrapAll(nil, (&SQLiteGrammar{}).WrapColumn))
}

// A column that is not named "<something>_id" gives nothing to infer from, so
// the name is used as the table rather than guessing. Callers who need
// something else say so with .On(...).
func TestInferForeignTableFallsBackToTheColumnName(t *testing.T) {
	assert.Equal(t, "owner", inferForeignTable("owner"))
	assert.Equal(t, "_id", inferForeignTable("_id"))

	bp := NewBlueprint("things")
	bp.Integer("owner")
	bp.Foreign("owner")

	assert.Contains(t, (&PostgresGrammar{}).CompileCreate(bp),
		`FOREIGN KEY ("owner") REFERENCES "owner" ("id")`)
}
