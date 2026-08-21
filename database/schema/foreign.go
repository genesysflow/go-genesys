package schema

import (
	"fmt"
	"strings"
)

// ForeignKeyDefinition is a fluent foreign key constraint:
//
//	table.ForeignID("user_id")
//	table.Foreign("user_id").References("id").On("users").CascadeOnDelete()
type ForeignKeyDefinition struct {
	Column         string
	RefTable       string
	RefColumn      string
	OnDeleteAction string
	OnUpdateAction string
}

// Foreign starts a foreign key constraint on the given column.
func (bp *Blueprint) Foreign(column string) *ForeignKeyDefinition {
	bp.foreignKeys = append(bp.foreignKeys, ForeignKeyDefinition{
		Column:    column,
		RefColumn: "id",
	})
	return &bp.foreignKeys[len(bp.foreignKeys)-1]
}

// References sets the referenced column (default "id").
func (fk *ForeignKeyDefinition) References(column string) *ForeignKeyDefinition {
	fk.RefColumn = column
	return fk
}

// On sets the referenced table.
func (fk *ForeignKeyDefinition) On(table string) *ForeignKeyDefinition {
	fk.RefTable = table
	return fk
}

// OnDelete sets the ON DELETE action ("cascade", "set null", "restrict",
// "no action").
func (fk *ForeignKeyDefinition) OnDelete(action string) *ForeignKeyDefinition {
	fk.OnDeleteAction = action
	return fk
}

// OnUpdate sets the ON UPDATE action.
func (fk *ForeignKeyDefinition) OnUpdate(action string) *ForeignKeyDefinition {
	fk.OnUpdateAction = action
	return fk
}

// CascadeOnDelete deletes dependent rows when the parent row is deleted.
func (fk *ForeignKeyDefinition) CascadeOnDelete() *ForeignKeyDefinition {
	return fk.OnDelete("cascade")
}

// NullOnDelete nulls the column when the parent row is deleted.
func (fk *ForeignKeyDefinition) NullOnDelete() *ForeignKeyDefinition {
	return fk.OnDelete("set null")
}

// RestrictOnDelete blocks deleting a parent row with dependents.
func (fk *ForeignKeyDefinition) RestrictOnDelete() *ForeignKeyDefinition {
	return fk.OnDelete("restrict")
}

// compileForeignKeys renders FOREIGN KEY table constraints; shared by all
// grammars (the syntax is standard SQL).
func compileForeignKeys(bp *Blueprint, wrap func(string) string) []string {
	var constraints []string
	for _, fk := range bp.foreignKeys {
		if fk.RefTable == "" {
			// Laravel-style inference: user_id -> users.id.
			fk.RefTable = inferForeignTable(fk.Column)
		}
		constraint := fmt.Sprintf("FOREIGN KEY (%s) REFERENCES %s (%s)",
			wrap(fk.Column), wrap(fk.RefTable), wrap(fk.RefColumn))
		if fk.OnDeleteAction != "" {
			constraint += " ON DELETE " + strings.ToUpper(fk.OnDeleteAction)
		}
		if fk.OnUpdateAction != "" {
			constraint += " ON UPDATE " + strings.ToUpper(fk.OnUpdateAction)
		}
		constraints = append(constraints, constraint)
	}
	return constraints
}

// inferForeignTable derives the referenced table from a column name:
// user_id -> users, blog_post_id -> blog_posts.
func inferForeignTable(column string) string {
	base := strings.TrimSuffix(column, "_id")
	if base == column || base == "" {
		return column
	}
	return pluralizeTable(base)
}

// pluralizeTable applies basic English pluralization for table inference.
func pluralizeTable(name string) string {
	switch {
	case strings.HasSuffix(name, "y") && len(name) > 1 && !strings.ContainsRune("aeiou", rune(name[len(name)-2])):
		return name[:len(name)-1] + "ies"
	case strings.HasSuffix(name, "s"), strings.HasSuffix(name, "x"),
		strings.HasSuffix(name, "z"), strings.HasSuffix(name, "ch"), strings.HasSuffix(name, "sh"):
		return name + "es"
	default:
		return name + "s"
	}
}
