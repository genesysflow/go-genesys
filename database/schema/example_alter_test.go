package schema_test

import (
	"database/sql"

	"github.com/genesysflow/go-genesys/database/schema"
)

// Example demonstrates how to use the ALTER table functionality
func Example_alterTable() {
	var db *sql.DB // Assume this is already initialized

	builder := schema.NewBuilder(db, "postgres")

	// First, create a table
	_ = builder.Create("users", func(table *schema.Blueprint) {
		table.ID()
		table.String("name", 100)
		table.String("email", 255).Unique()
		table.Timestamps()
	})

	// Later, in a migration, alter the table
	_ = builder.Table("users", func(table *schema.Blueprint) {
		// Add new columns
		table.AddString("phone", 20).Nullable()
		table.AddBoolean("verified").Default(false)
		table.AddTimestamp("last_login_at").Nullable()
	})
}

// Example demonstrates dropping columns
func Example_dropColumns() {
	var db *sql.DB // Assume this is already initialized

	builder := schema.NewBuilder(db, "postgres")

	_ = builder.Table("users", func(table *schema.Blueprint) {
		// Drop single or multiple columns
		table.DropColumn("old_column")
		table.DropColumn("deprecated_field", "unused_field")
	})
}

// Example demonstrates renaming columns
func Example_renameColumn() {
	var db *sql.DB // Assume this is already initialized

	builder := schema.NewBuilder(db, "postgres")

	_ = builder.Table("users", func(table *schema.Blueprint) {
		// Rename a column
		table.RenameColumn("email_address", "email")
	})
}

// Example demonstrates modifying column definitions
func Example_modifyColumn() {
	var db *sql.DB // Assume this is already initialized

	builder := schema.NewBuilder(db, "postgres")

	_ = builder.Table("users", func(table *schema.Blueprint) {
		// Modify column - requires full redefinition
		table.ModifyColumn("status").String(50).Default("active")
		
		// Change column type and nullability
		table.ModifyColumn("age").Integer().Nullable()
	})
}

// Example demonstrates multiple operations in one migration
func Example_multipleOperations() {
	var db *sql.DB // Assume this is already initialized

	builder := schema.NewBuilder(db, "postgres")

	_ = builder.Table("users", func(table *schema.Blueprint) {
		// Add new columns
		table.AddString("first_name", 100)
		table.AddString("last_name", 100)
		
		// Drop old column
		table.DropColumn("name")
		
		// Rename column
		table.RenameColumn("old_email", "email")
		
		// Modify existing column
		table.ModifyColumn("status").String(50).Default("active")
		
		// Drop indexes
		table.DropIndex("old_index_column")
		table.DropUnique("old_unique_column")
	})
}
