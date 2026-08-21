package schema

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestForeignKeyCompilation(t *testing.T) {
	bp := NewBlueprint("posts")
	bp.ID()
	bp.ForeignID("user_id")
	bp.String("title", 255)
	bp.Foreign("user_id").References("id").On("users").CascadeOnDelete()

	for _, grammar := range []Grammar{&SQLiteGrammar{}, &PostgresGrammar{}} {
		sql := grammar.CompileCreate(bp)
		assert.Contains(t, sql, "FOREIGN KEY")
		assert.Contains(t, sql, `REFERENCES "users" ("id")`)
		assert.Contains(t, sql, "ON DELETE CASCADE")
	}
}

func TestForeignKeyTableInference(t *testing.T) {
	bp := NewBlueprint("comments")
	bp.ID()
	bp.ForeignID("blog_post_id")
	bp.Foreign("blog_post_id").NullOnDelete()

	sql := (&SQLiteGrammar{}).CompileCreate(bp)
	assert.Contains(t, sql, `REFERENCES "blog_posts" ("id")`)
	assert.Contains(t, sql, "ON DELETE SET NULL")
}

func TestForeignKeyOnUpdate(t *testing.T) {
	bp := NewBlueprint("orders")
	bp.ID()
	bp.ForeignID("category_id")
	bp.Foreign("category_id").On("categories").RestrictOnDelete().OnUpdate("cascade")

	sql := (&PostgresGrammar{}).CompileCreate(bp)
	assert.Contains(t, sql, "ON DELETE RESTRICT")
	assert.Contains(t, sql, "ON UPDATE CASCADE")
	// Constraint comes after all columns.
	assert.True(t, strings.Index(sql, "FOREIGN KEY") > strings.Index(sql, "category_id"))
}

func TestPluralizeTableInference(t *testing.T) {
	assert.Equal(t, "users", inferForeignTable("user_id"))
	assert.Equal(t, "categories", inferForeignTable("category_id"))
	assert.Equal(t, "boxes", inferForeignTable("box_id"))
	assert.Equal(t, "statuses", inferForeignTable("status_id"))
}
