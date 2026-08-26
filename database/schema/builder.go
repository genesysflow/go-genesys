// Package schema provides a fluent schema builder for database migrations.
//
// The schema builder supports both creating new tables and altering existing ones.
//
// Example - Creating a table:
//
//	builder.Create("users", func(table *schema.Blueprint) {
//		table.ID()
//		table.String("name", 100)
//		table.String("email", 255).Unique()
//		table.Timestamps()
//	})
//
// Example - Altering a table:
//
//	builder.Table("users", func(table *schema.Blueprint) {
//		// Add new columns
//		table.AddString("phone", 20).Nullable()
//		table.AddBoolean("verified").Default(false)
//
//		// Drop columns
//		table.DropColumn("old_column")
//
//		// Rename columns
//		table.RenameColumn("email_address", "email")
//
//		// Modify column definition (requires full redefinition)
//		table.ModifyColumn("status").String(50).Default("active")
//
//		// Drop indexes
//		table.DropIndex("column_name")
//		table.DropUnique("column_name")
//	})
//
// Table only understands the Add*/Drop*/RenameColumn/ModifyColumn commands; using the
// create-style methods (String, ID, Unique, ...) inside it returns an error instead of
// silently applying nothing. Operations a driver cannot express — modifying a column,
// dropping a UNIQUE constraint or a primary key on SQLite — return ErrUnsupportedOperation
// before any statement runs.
package schema

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// ErrUnsupportedOperation is returned when a schema operation cannot be expressed
// in SQL for the current driver (for example, modifying a column type on SQLite).
// Callers can test for it with errors.Is.
var ErrUnsupportedOperation = errors.New("schema: unsupported operation")

// Builder provides fluent schema building.
type Builder struct {
	db      *sql.DB
	grammar Grammar
}

// NewBuilder creates a new schema builder.
func NewBuilder(db *sql.DB, driver string) *Builder {
	return &Builder{
		db:      db,
		grammar: NewGrammar(driver),
	}
}

// Create creates a new table, then any secondary indexes declared on
// the blueprint (table-level Index(...) or column-level .Index()).
func (b *Builder) Create(table string, callback func(*Blueprint)) error {
	bp := NewBlueprint(table)
	bp.create = true
	callback(bp)

	if _, err := b.db.Exec(b.grammar.CompileCreate(bp)); err != nil {
		return err
	}
	for _, stmt := range b.grammar.CompileCreateIndexes(bp) {
		if _, err := b.db.Exec(stmt); err != nil {
			return err
		}
	}
	// Dialects that keep column comments outside the column definition
	// (PostgreSQL) attach them here; the others returned nothing to run.
	for _, stmt := range b.grammar.CompileComments(bp) {
		if _, err := b.db.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

// Table modifies an existing table.
// All ALTER statements are executed within a transaction for atomicity.
func (b *Builder) Table(table string, callback func(*Blueprint)) error {
	bp := NewBlueprint(table)
	callback(bp)

	// Create-style methods (String, ID, Index, Unique, ...) populate columns/indexes,
	// which CompileAlter never looks at. Committing an empty migration would silently
	// do nothing, so fail with an actionable message instead.
	if len(bp.columns) > 0 || len(bp.indexes) > 0 {
		return fmt.Errorf("schema: create-style column/index methods (e.g. String, ID, Unique) cannot be used in Table migrations on %q; use Add*/Drop*/RenameColumn/ModifyColumn instead", table)
	}

	// Compile all ALTER commands
	sqls, err := b.grammar.CompileAlter(bp)
	if err != nil {
		return err
	}

	// Begin transaction for atomic schema changes
	tx, err := b.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Execute each ALTER statement
	for _, sql := range sqls {
		if sql == "" {
			continue // Skip empty statements
		}
		if _, err := tx.Exec(sql); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// Drop drops a table.
func (b *Builder) Drop(table string) error {
	sql := fmt.Sprintf("DROP TABLE %s", b.grammar.WrapTable(table))
	_, err := b.db.Exec(sql)
	return err
}

// DropIfExists drops a table if it exists.
func (b *Builder) DropIfExists(table string) error {
	sql := fmt.Sprintf("DROP TABLE IF EXISTS %s", b.grammar.WrapTable(table))
	_, err := b.db.Exec(sql)
	return err
}

// Rename renames a table.
func (b *Builder) Rename(from, to string) error {
	sql := fmt.Sprintf("ALTER TABLE %s RENAME TO %s", b.grammar.WrapTable(from), b.grammar.WrapTable(to))
	_, err := b.db.Exec(sql)
	return err
}

// HasTable checks if a table exists.
func (b *Builder) HasTable(table string) bool {
	sql := b.grammar.CompileTableExists(table)
	var result int
	err := b.db.QueryRow(sql).Scan(&result)
	return err == nil && result > 0
}

// Blueprint defines a table structure.
type Blueprint struct {
	table       string
	columns     []ColumnDefinition
	indexes     []IndexDefinition
	foreignKeys []ForeignKeyDefinition
	create      bool
	commands    []AlterCommand // For ALTER table operations
}

// NewBlueprint creates a new blueprint.
func NewBlueprint(table string) *Blueprint {
	return &Blueprint{
		table:    table,
		columns:  make([]ColumnDefinition, 0),
		indexes:  make([]IndexDefinition, 0),
		commands: make([]AlterCommand, 0),
	}
}

// ColumnDefinition represents a column definition.
type ColumnDefinition struct {
	Name                  string
	Type                  string
	Length                int
	Precision             int
	Scale                 int
	IsNullable            bool
	NullableExplicitlySet bool // Track if nullable was explicitly set for ModifyColumn
	DefaultValue          any
	DefaultExplicitlySet  bool // Track if default was explicitly set for ModifyColumn
	AutoIncrement         bool
	Primary               bool
	IsUnique              bool
	IsIndex               bool
	Unsigned              bool
	ColumnComment         string
}

// IndexDefinition represents an index definition.
type IndexDefinition struct {
	Name    string
	Columns []string
	Type    string // PRIMARY, UNIQUE, INDEX
}

// AlterCommand represents an ALTER table operation.
type AlterCommand struct {
	Type     string // "add", "drop", "rename", "modify", "dropIndex", "dropUnique", "dropPrimary"
	Column   *ColumnDefinition
	OldName  string   // For rename operations
	NewName  string   // For rename operations
	Columns  []string // For drop operations and index operations
	IndexDef *IndexDefinition
}

// ID adds an auto-incrementing primary key column.
func (bp *Blueprint) ID(name ...string) *ColumnDefinition {
	colName := "id"
	if len(name) > 0 {
		colName = name[0]
	}
	col := ColumnDefinition{
		Name:          colName,
		Type:          "integer",
		AutoIncrement: true,
		Primary:       true,
	}
	bp.columns = append(bp.columns, col)
	return &bp.columns[len(bp.columns)-1]
}

// BigIncrements adds an auto-incrementing big integer primary key.
func (bp *Blueprint) BigIncrements(name string) *ColumnDefinition {
	col := ColumnDefinition{
		Name:          name,
		Type:          "bigint",
		AutoIncrement: true,
		Primary:       true,
	}
	bp.columns = append(bp.columns, col)
	return &bp.columns[len(bp.columns)-1]
}

// String adds a VARCHAR column.
func (bp *Blueprint) String(name string, length ...int) *ColumnDefinition {
	l := 255
	if len(length) > 0 {
		l = length[0]
	}
	col := ColumnDefinition{
		Name:   name,
		Type:   "varchar",
		Length: l,
	}
	bp.columns = append(bp.columns, col)
	return &bp.columns[len(bp.columns)-1]
}

// Text adds a TEXT column.
func (bp *Blueprint) Text(name string) *ColumnDefinition {
	col := ColumnDefinition{
		Name: name,
		Type: "text",
	}
	bp.columns = append(bp.columns, col)
	return &bp.columns[len(bp.columns)-1]
}

// Integer adds an INTEGER column.
func (bp *Blueprint) Integer(name string) *ColumnDefinition {
	col := ColumnDefinition{
		Name: name,
		Type: "integer",
	}
	bp.columns = append(bp.columns, col)
	return &bp.columns[len(bp.columns)-1]
}

// BigInteger adds a BIGINT column.
func (bp *Blueprint) BigInteger(name string) *ColumnDefinition {
	col := ColumnDefinition{
		Name: name,
		Type: "bigint",
	}
	bp.columns = append(bp.columns, col)
	return &bp.columns[len(bp.columns)-1]
}

// Boolean adds a BOOLEAN column.
func (bp *Blueprint) Boolean(name string) *ColumnDefinition {
	col := ColumnDefinition{
		Name: name,
		Type: "boolean",
	}
	bp.columns = append(bp.columns, col)
	return &bp.columns[len(bp.columns)-1]
}

// Decimal adds a DECIMAL column.
func (bp *Blueprint) Decimal(name string, precision, scale int) *ColumnDefinition {
	col := ColumnDefinition{
		Name:      name,
		Type:      "decimal",
		Precision: precision,
		Scale:     scale,
	}
	bp.columns = append(bp.columns, col)
	return &bp.columns[len(bp.columns)-1]
}

// Float adds a FLOAT column.
func (bp *Blueprint) Float(name string) *ColumnDefinition {
	col := ColumnDefinition{
		Name: name,
		Type: "float",
	}
	bp.columns = append(bp.columns, col)
	return &bp.columns[len(bp.columns)-1]
}

// DateTime adds a DATETIME column.
func (bp *Blueprint) DateTime(name string) *ColumnDefinition {
	col := ColumnDefinition{
		Name: name,
		Type: "datetime",
	}
	bp.columns = append(bp.columns, col)
	return &bp.columns[len(bp.columns)-1]
}

// Timestamp adds a TIMESTAMP column.
func (bp *Blueprint) Timestamp(name string) *ColumnDefinition {
	col := ColumnDefinition{
		Name: name,
		Type: "timestamp",
	}
	bp.columns = append(bp.columns, col)
	return &bp.columns[len(bp.columns)-1]
}

// Timestamps adds created_at and updated_at columns.
func (bp *Blueprint) Timestamps() {
	bp.Timestamp("created_at").Nullable()
	bp.Timestamp("updated_at").Nullable()
}

// SoftDeletes adds a deleted_at column for soft deletes.
func (bp *Blueprint) SoftDeletes() {
	bp.Timestamp("deleted_at").Nullable()
}

// ForeignID adds a foreign key column.
func (bp *Blueprint) ForeignID(name string) *ColumnDefinition {
	col := ColumnDefinition{
		Name:     name,
		Type:     "bigint",
		Unsigned: true,
	}
	bp.columns = append(bp.columns, col)
	return &bp.columns[len(bp.columns)-1]
}

// Index adds an index.
func (bp *Blueprint) Index(columns ...string) {
	bp.indexes = append(bp.indexes, IndexDefinition{
		Columns: columns,
		Type:    "INDEX",
	})
}

// Unique adds a unique index.
func (bp *Blueprint) Unique(columns ...string) {
	bp.indexes = append(bp.indexes, IndexDefinition{
		Columns: columns,
		Type:    "UNIQUE",
	})
}

// Primary adds a primary key.
func (bp *Blueprint) Primary(columns ...string) {
	bp.indexes = append(bp.indexes, IndexDefinition{
		Columns: columns,
		Type:    "PRIMARY",
	})
}

// AddColumn adds a new column to an existing table (for ALTER operations).
func (bp *Blueprint) AddColumn(name, colType string) *ColumnDefinition {
	col := ColumnDefinition{
		Name: name,
		Type: colType,
	}
	bp.commands = append(bp.commands, AlterCommand{
		Type:   "add",
		Column: &col,
	})
	// Return pointer to the column in the command
	return bp.commands[len(bp.commands)-1].Column
}

// AddString adds a VARCHAR column to an existing table.
func (bp *Blueprint) AddString(name string, length ...int) *ColumnDefinition {
	l := 255
	if len(length) > 0 {
		l = length[0]
	}
	col := ColumnDefinition{
		Name:   name,
		Type:   "varchar",
		Length: l,
	}
	bp.commands = append(bp.commands, AlterCommand{
		Type:   "add",
		Column: &col,
	})
	return bp.commands[len(bp.commands)-1].Column
}

// AddInteger adds an INTEGER column to an existing table.
func (bp *Blueprint) AddInteger(name string) *ColumnDefinition {
	col := ColumnDefinition{
		Name: name,
		Type: "integer",
	}
	bp.commands = append(bp.commands, AlterCommand{
		Type:   "add",
		Column: &col,
	})
	return bp.commands[len(bp.commands)-1].Column
}

// AddBigInteger adds a BIGINT column to an existing table.
func (bp *Blueprint) AddBigInteger(name string) *ColumnDefinition {
	col := ColumnDefinition{
		Name: name,
		Type: "bigint",
	}
	bp.commands = append(bp.commands, AlterCommand{
		Type:   "add",
		Column: &col,
	})
	return bp.commands[len(bp.commands)-1].Column
}

// AddText adds a TEXT column to an existing table.
func (bp *Blueprint) AddText(name string) *ColumnDefinition {
	col := ColumnDefinition{
		Name: name,
		Type: "text",
	}
	bp.commands = append(bp.commands, AlterCommand{
		Type:   "add",
		Column: &col,
	})
	return bp.commands[len(bp.commands)-1].Column
}

// AddBoolean adds a BOOLEAN column to an existing table.
func (bp *Blueprint) AddBoolean(name string) *ColumnDefinition {
	col := ColumnDefinition{
		Name: name,
		Type: "boolean",
	}
	bp.commands = append(bp.commands, AlterCommand{
		Type:   "add",
		Column: &col,
	})
	return bp.commands[len(bp.commands)-1].Column
}

// AddTimestamp adds a TIMESTAMP column to an existing table.
func (bp *Blueprint) AddTimestamp(name string) *ColumnDefinition {
	col := ColumnDefinition{
		Name: name,
		Type: "timestamp",
	}
	bp.commands = append(bp.commands, AlterCommand{
		Type:   "add",
		Column: &col,
	})
	return bp.commands[len(bp.commands)-1].Column
}

// AddDecimal adds a DECIMAL column to an existing table.
func (bp *Blueprint) AddDecimal(name string, precision, scale int) *ColumnDefinition {
	col := ColumnDefinition{
		Name:      name,
		Type:      "decimal",
		Precision: precision,
		Scale:     scale,
	}
	bp.commands = append(bp.commands, AlterCommand{
		Type:   "add",
		Column: &col,
	})
	return bp.commands[len(bp.commands)-1].Column
}

// AddFloat adds a FLOAT column to an existing table.
func (bp *Blueprint) AddFloat(name string) *ColumnDefinition {
	col := ColumnDefinition{
		Name: name,
		Type: "float",
	}
	bp.commands = append(bp.commands, AlterCommand{
		Type:   "add",
		Column: &col,
	})
	return bp.commands[len(bp.commands)-1].Column
}

// AddDateTime adds a DATETIME column to an existing table.
func (bp *Blueprint) AddDateTime(name string) *ColumnDefinition {
	col := ColumnDefinition{
		Name: name,
		Type: "datetime",
	}
	bp.commands = append(bp.commands, AlterCommand{
		Type:   "add",
		Column: &col,
	})
	return bp.commands[len(bp.commands)-1].Column
}

// DropColumn drops one or more columns from an existing table.
func (bp *Blueprint) DropColumn(columns ...string) {
	bp.commands = append(bp.commands, AlterCommand{
		Type:    "drop",
		Columns: columns,
	})
}

// RenameColumn renames a column.
func (bp *Blueprint) RenameColumn(from, to string) {
	bp.commands = append(bp.commands, AlterCommand{
		Type:    "rename",
		OldName: from,
		NewName: to,
	})
}

// ModifyColumn modifies an existing column's definition.
// Returns ColumnDefinition for chaining the new definition.
func (bp *Blueprint) ModifyColumn(name string) *ColumnDefinition {
	col := ColumnDefinition{
		Name: name,
	}
	bp.commands = append(bp.commands, AlterCommand{
		Type:   "modify",
		Column: &col,
	})
	return bp.commands[len(bp.commands)-1].Column
}

// DropIndex drops an index.
func (bp *Blueprint) DropIndex(columns ...string) {
	bp.commands = append(bp.commands, AlterCommand{
		Type:    "dropIndex",
		Columns: columns,
	})
}

// DropUnique drops a unique constraint.
func (bp *Blueprint) DropUnique(columns ...string) {
	bp.commands = append(bp.commands, AlterCommand{
		Type:    "dropUnique",
		Columns: columns,
	})
}

// DropPrimary drops the primary key.
func (bp *Blueprint) DropPrimary() {
	bp.commands = append(bp.commands, AlterCommand{
		Type: "dropPrimary",
	})
}

// Column methods for fluent configuration
func (c *ColumnDefinition) Nullable() *ColumnDefinition {
	c.IsNullable = true
	c.NullableExplicitlySet = true
	return c
}

func (c *ColumnDefinition) Default(value any) *ColumnDefinition {
	c.DefaultValue = value
	c.DefaultExplicitlySet = true
	return c
}

func (c *ColumnDefinition) Unique() *ColumnDefinition {
	c.IsUnique = true
	return c
}

func (c *ColumnDefinition) Index() *ColumnDefinition {
	c.IsIndex = true
	return c
}

func (c *ColumnDefinition) Comment(comment string) *ColumnDefinition {
	c.ColumnComment = comment
	return c
}

// Type setter methods for ModifyColumn support
func (c *ColumnDefinition) String(length ...int) *ColumnDefinition {
	c.Type = "varchar"
	c.Length = 255
	if len(length) > 0 {
		c.Length = length[0]
	}
	return c
}

func (c *ColumnDefinition) Text() *ColumnDefinition {
	c.Type = "text"
	return c
}

func (c *ColumnDefinition) Integer() *ColumnDefinition {
	c.Type = "integer"
	return c
}

func (c *ColumnDefinition) BigInteger() *ColumnDefinition {
	c.Type = "bigint"
	return c
}

func (c *ColumnDefinition) Boolean() *ColumnDefinition {
	c.Type = "boolean"
	return c
}

func (c *ColumnDefinition) Decimal(precision, scale int) *ColumnDefinition {
	c.Type = "decimal"
	c.Precision = precision
	c.Scale = scale
	return c
}

func (c *ColumnDefinition) Float() *ColumnDefinition {
	c.Type = "float"
	return c
}

func (c *ColumnDefinition) DateTime() *ColumnDefinition {
	c.Type = "datetime"
	return c
}

func (c *ColumnDefinition) Timestamp() *ColumnDefinition {
	c.Type = "timestamp"
	return c
}

// Grammar compiles schema to SQL.
type Grammar interface {
	CompileCreate(bp *Blueprint) string
	CompileCreateIndexes(bp *Blueprint) []string
	// CompileComments returns the statements that attach column comments once
	// the table exists. Dialects that write the comment inline in the column
	// definition (MySQL) or have no column comments at all (SQLite) return none.
	CompileComments(bp *Blueprint) []string
	CompileTableExists(table string) string
	WrapTable(table string) string
	WrapColumn(column string) string
	CompileAlter(bp *Blueprint) ([]string, error)
	CompileAddColumn(table string, col ColumnDefinition) string
	CompileDropColumn(table string, column string) string
	CompileRenameColumn(table, from, to string) string
	CompileModifyColumn(table string, col ColumnDefinition) ([]string, error)
	CompileDropIndex(table string, columns []string) string
	CompileDropUnique(table string, columns []string) ([]string, error)
	CompileDropPrimary(table string) (string, error)
}

// NewGrammar creates a grammar for the given driver.
func NewGrammar(driver string) Grammar {
	switch driver {
	case "pgsql", "postgres", "postgresql":
		return &PostgresGrammar{}
	case "mariadb":
		return &MySQLGrammar{MariaDB: true}
	case "mysql":
		return &MySQLGrammar{}
	default:
		return &SQLiteGrammar{}
	}
}

// columnOptions carries the context a lone ColumnDefinition cannot know about
// itself, so compileColumn can render the same column differently depending on
// the statement it lands in.
type columnOptions struct {
	// compositePrimary is set when the blueprint flags more than one
	// column-level primary key. CompileCreate then declares the key once, as a
	// table-level PRIMARY KEY (a, b) constraint, and the inline PRIMARY KEY each
	// column would otherwise carry has to be suppressed: no dialect accepts a
	// table with several primary keys.
	compositePrimary bool

	// modify is set when the definition is rendered for MySQL's MODIFY COLUMN,
	// which replaces the column wholesale rather than amending it.
	modify bool
}

// columnPrimaryKeys returns the wrapped names of the columns carrying a
// column-level Primary flag but no auto-increment — the ones whose key has to
// be declared as a table constraint. More than one means a composite primary
// key. Blueprint.Primary(...) takes the separate table-level path in
// compileTableConstraints and is not counted here.
func columnPrimaryKeys(bp *Blueprint, wrap func(string) string) []string {
	var keys []string
	for _, col := range bp.columns {
		if col.Primary && !col.AutoIncrement {
			keys = append(keys, wrap(col.Name))
		}
	}
	return keys
}

// wrapAll wraps each identifier in a list.
func wrapAll(columns []string, wrap func(string) string) []string {
	wrapped := make([]string, len(columns))
	for i, c := range columns {
		wrapped[i] = wrap(c)
	}
	return wrapped
}

// compileTableConstraints renders bp's table-level UNIQUE and PRIMARY
// entries as CREATE TABLE constraint parts. Unique constraints are
// named "<table>_<cols>_unique" so DropUnique can find them on every
// driver.
func compileTableConstraints(bp *Blueprint, wrap func(string) string) []string {
	var parts []string
	for _, idx := range bp.indexes {
		joined := strings.Join(wrapAll(idx.Columns, wrap), ", ")
		switch idx.Type {
		case "UNIQUE":
			name := bp.table + "_" + strings.Join(idx.Columns, "_") + "_unique"
			parts = append(parts, fmt.Sprintf("CONSTRAINT %s UNIQUE (%s)", wrap(name), joined))
		case "PRIMARY":
			parts = append(parts, fmt.Sprintf("PRIMARY KEY (%s)", joined))
		}
	}
	return parts
}

// compileCreateIndexStatements renders CREATE INDEX statements for bp's
// table-level Index(...) entries and column-level .Index() modifiers,
// named "<table>_<cols>_index" to match CompileDropIndex.
func compileCreateIndexStatements(bp *Blueprint, wrapTable, wrapIdent func(string) string) []string {
	var stmts []string
	add := func(columns []string) {
		name := bp.table + "_" + strings.Join(columns, "_") + "_index"
		stmts = append(stmts, fmt.Sprintf("CREATE INDEX %s ON %s (%s)",
			wrapIdent(name), wrapTable(bp.table), strings.Join(wrapAll(columns, wrapIdent), ", ")))
	}
	for _, col := range bp.columns {
		if col.IsIndex {
			add([]string{col.Name})
		}
	}
	for _, idx := range bp.indexes {
		if idx.Type == "INDEX" {
			add(idx.Columns)
		}
	}
	return stmts
}

// quoteIdentifier wraps a SQL identifier in double quotes, escaping any quote
// it contains by doubling it, as the SQL standard requires.
//
// Identifiers cannot be passed as bind parameters, so they are necessarily
// interpolated. Without escaping, an identifier containing a double quote
// would terminate the quoted region early and let the remainder of the string
// be parsed as SQL — table and column names normally come from migration
// source, but any that reaches this from configuration or user input would be
// an injection point.
func quoteIdentifier(identifier string) string {
	return `"` + strings.ReplaceAll(identifier, `"`, `""`) + `"`
}

// quoteString renders a value as a single-quoted SQL string literal, escaping
// embedded single quotes.
func quoteString(value string) string {
	return `'` + strings.ReplaceAll(value, `'`, `''`) + `'`
}

// SQLiteGrammar compiles schema for SQLite.
type SQLiteGrammar struct{}

func (g *SQLiteGrammar) WrapTable(table string) string {
	return quoteIdentifier(table)
}

func (g *SQLiteGrammar) WrapColumn(column string) string {
	return quoteIdentifier(column)
}

func (g *SQLiteGrammar) CompileTableExists(table string) string {
	return fmt.Sprintf("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=%s", quoteString(table))
}

func (g *SQLiteGrammar) CompileCreate(bp *Blueprint) string {
	var parts []string

	primaryKeys := columnPrimaryKeys(bp, g.WrapColumn)
	opts := columnOptions{compositePrimary: len(primaryKeys) > 1}

	for _, col := range bp.columns {
		parts = append(parts, g.compileColumn(col, opts))
	}

	// A composite key is declared once, as a table constraint; compileColumn
	// left the inline PRIMARY KEY off each of its columns.
	if opts.compositePrimary {
		parts = append(parts, fmt.Sprintf("PRIMARY KEY (%s)", strings.Join(primaryKeys, ", ")))
	}

	parts = append(parts, compileTableConstraints(bp, g.WrapColumn)...)
	parts = append(parts, compileForeignKeys(bp, g.WrapColumn)...)

	return fmt.Sprintf("CREATE TABLE %s (\n  %s\n)", g.WrapTable(bp.table), strings.Join(parts, ",\n  "))
}

// CompileCreateIndexes compiles the CREATE INDEX statements that follow
// a table's creation.
func (g *SQLiteGrammar) CompileCreateIndexes(bp *Blueprint) []string {
	return compileCreateIndexStatements(bp, g.WrapTable, g.WrapColumn)
}

// CompileComments returns nothing: SQLite has no column comments, so a
// blueprint's Comment(...) is silently ignored on this driver.
func (g *SQLiteGrammar) CompileComments(bp *Blueprint) []string {
	return nil
}

func (g *SQLiteGrammar) compileColumn(col ColumnDefinition, opts columnOptions) string {
	var def strings.Builder

	def.WriteString(g.WrapColumn(col.Name))
	def.WriteString(" ")

	// Type
	switch col.Type {
	case "varchar":
		def.WriteString(fmt.Sprintf("VARCHAR(%d)", col.Length))
	case "decimal":
		def.WriteString(fmt.Sprintf("DECIMAL(%d,%d)", col.Precision, col.Scale))
	case "bigint":
		def.WriteString("INTEGER") // SQLite uses INTEGER for all ints
	case "integer":
		def.WriteString("INTEGER")
	default:
		def.WriteString(strings.ToUpper(col.Type))
	}

	// Primary key with autoincrement
	if col.Primary && col.AutoIncrement {
		def.WriteString(" PRIMARY KEY AUTOINCREMENT")
	} else if col.Primary && !opts.compositePrimary {
		def.WriteString(" PRIMARY KEY")
	}

	// Not null
	if !col.IsNullable && !col.Primary {
		def.WriteString(" NOT NULL")
	}

	// Unique
	if col.IsUnique {
		def.WriteString(" UNIQUE")
	}

	// Default
	if col.DefaultValue != nil {
		switch v := col.DefaultValue.(type) {
		case string:
			def.WriteString(" DEFAULT " + quoteString(v))
		case bool:
			if v {
				def.WriteString(" DEFAULT 1")
			} else {
				def.WriteString(" DEFAULT 0")
			}
		default:
			def.WriteString(fmt.Sprintf(" DEFAULT %v", v))
		}
	}

	return def.String()
}

// CompileAlter compiles ALTER table commands for SQLite.
// It returns an error for operations SQLite cannot express, so callers fail fast
// instead of committing a migration that changed nothing.
func (g *SQLiteGrammar) CompileAlter(bp *Blueprint) ([]string, error) {
	var statements []string

	for _, cmd := range bp.commands {
		switch cmd.Type {
		case "add":
			statements = append(statements, g.CompileAddColumn(bp.table, *cmd.Column))
		case "drop":
			// SQLite requires separate statements for each dropped column
			for _, col := range cmd.Columns {
				statements = append(statements, g.CompileDropColumn(bp.table, col))
			}
		case "rename":
			statements = append(statements, g.CompileRenameColumn(bp.table, cmd.OldName, cmd.NewName))
		case "modify":
			stmts, err := g.CompileModifyColumn(bp.table, *cmd.Column)
			if err != nil {
				return nil, err
			}
			statements = append(statements, stmts...)
		case "dropIndex":
			statements = append(statements, g.CompileDropIndex(bp.table, cmd.Columns))
		case "dropUnique":
			stmts, err := g.CompileDropUnique(bp.table, cmd.Columns)
			if err != nil {
				return nil, err
			}
			statements = append(statements, stmts...)
		case "dropPrimary":
			stmt, err := g.CompileDropPrimary(bp.table)
			if err != nil {
				return nil, err
			}
			statements = append(statements, stmt)
		}
	}

	return statements, nil
}

// CompileAddColumn compiles ADD COLUMN statement for SQLite.
func (g *SQLiteGrammar) CompileAddColumn(table string, col ColumnDefinition) string {
	colDef := g.compileColumn(col, columnOptions{})
	return fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s", g.WrapTable(table), colDef)
}

// CompileDropColumn compiles DROP COLUMN statement for SQLite.
func (g *SQLiteGrammar) CompileDropColumn(table string, column string) string {
	// SQLite 3.35+ supports DROP COLUMN
	// For older versions, this will fail and user needs to recreate table
	return fmt.Sprintf("ALTER TABLE %s DROP COLUMN %s", g.WrapTable(table), g.WrapColumn(column))
}

// CompileRenameColumn compiles RENAME COLUMN statement for SQLite.
func (g *SQLiteGrammar) CompileRenameColumn(table, from, to string) string {
	return fmt.Sprintf("ALTER TABLE %s RENAME COLUMN %s TO %s",
		g.WrapTable(table), g.WrapColumn(from), g.WrapColumn(to))
}

// CompileModifyColumn compiles ALTER COLUMN statement for SQLite.
// SQLite has no ALTER COLUMN, so this always reports ErrUnsupportedOperation.
func (g *SQLiteGrammar) CompileModifyColumn(table string, col ColumnDefinition) ([]string, error) {
	return nil, fmt.Errorf("%w: SQLite cannot modify column %s.%s; recreate the table instead",
		ErrUnsupportedOperation, table, col.Name)
}

// CompileDropIndex compiles DROP INDEX statement for SQLite.
func (g *SQLiteGrammar) CompileDropIndex(table string, columns []string) string {
	// SQLite uses named indexes, construct index name from table and columns
	indexName := table + "_" + strings.Join(columns, "_") + "_index"
	return fmt.Sprintf("DROP INDEX IF EXISTS %s", g.WrapColumn(indexName))
}

// CompileDropUnique compiles DROP UNIQUE constraint for SQLite.
// Inline UNIQUE constraints are backed by auto-generated sqlite_autoindex_* names that
// cannot be derived from the table/column names, so this reports ErrUnsupportedOperation.
func (g *SQLiteGrammar) CompileDropUnique(table string, columns []string) ([]string, error) {
	return nil, fmt.Errorf("%w: SQLite cannot drop the UNIQUE constraint on %s(%s); recreate the table without it",
		ErrUnsupportedOperation, table, strings.Join(columns, ", "))
}

// CompileDropPrimary compiles DROP PRIMARY KEY for SQLite.
// SQLite has no way to drop a primary key, so this reports ErrUnsupportedOperation.
func (g *SQLiteGrammar) CompileDropPrimary(table string) (string, error) {
	return "", fmt.Errorf("%w: SQLite cannot drop the primary key on %s; recreate the table instead",
		ErrUnsupportedOperation, table)
}

// PostgresGrammar compiles schema for PostgreSQL.
type PostgresGrammar struct{}

func (g *PostgresGrammar) WrapTable(table string) string {
	return quoteIdentifier(table)
}

func (g *PostgresGrammar) WrapColumn(column string) string {
	return quoteIdentifier(column)
}

// CompileTableExists counts the table in the schema the connection resolves
// unqualified names against. Without the current_schema() predicate the query
// matches any schema in the database, so HasTable("users") would report true
// for an archive.users the connection cannot reach.
func (g *PostgresGrammar) CompileTableExists(table string) string {
	return fmt.Sprintf(
		"SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = current_schema() AND table_name = %s",
		quoteString(table))
}

func (g *PostgresGrammar) CompileCreate(bp *Blueprint) string {
	var parts []string

	primaryKeys := columnPrimaryKeys(bp, g.WrapColumn)
	opts := columnOptions{compositePrimary: len(primaryKeys) > 1}

	for _, col := range bp.columns {
		parts = append(parts, g.compileColumn(col, opts))
	}

	// A composite key is declared once, as a table constraint; compileColumn
	// left the inline PRIMARY KEY off each of its columns.
	if opts.compositePrimary {
		parts = append(parts, fmt.Sprintf("PRIMARY KEY (%s)", strings.Join(primaryKeys, ", ")))
	}

	parts = append(parts, compileTableConstraints(bp, g.WrapColumn)...)
	parts = append(parts, compileForeignKeys(bp, g.WrapColumn)...)

	return fmt.Sprintf("CREATE TABLE %s (\n  %s\n)", g.WrapTable(bp.table), strings.Join(parts, ",\n  "))
}

// CompileCreateIndexes compiles the CREATE INDEX statements that follow
// a table's creation.
func (g *PostgresGrammar) CompileCreateIndexes(bp *Blueprint) []string {
	return compileCreateIndexStatements(bp, g.WrapTable, g.WrapColumn)
}

// CompileComments returns one COMMENT ON COLUMN statement per commented column.
// PostgreSQL has no inline column comment, so these run after the CREATE TABLE.
func (g *PostgresGrammar) CompileComments(bp *Blueprint) []string {
	var stmts []string
	for _, col := range bp.columns {
		if col.ColumnComment == "" {
			continue
		}
		stmts = append(stmts, g.compileComment(bp.table, col))
	}
	return stmts
}

// compileComment renders the COMMENT ON COLUMN statement for one column.
func (g *PostgresGrammar) compileComment(table string, col ColumnDefinition) string {
	return fmt.Sprintf("COMMENT ON COLUMN %s.%s IS %s",
		g.WrapTable(table), g.WrapColumn(col.Name), quoteString(col.ColumnComment))
}

func (g *PostgresGrammar) compileColumn(col ColumnDefinition, opts columnOptions) string {
	var def strings.Builder

	def.WriteString(g.WrapColumn(col.Name))
	def.WriteString(" ")

	// Type
	if col.AutoIncrement {
		if col.Type == "bigint" {
			def.WriteString("BIGSERIAL")
		} else {
			def.WriteString("SERIAL")
		}
	} else {
		switch col.Type {
		case "varchar":
			def.WriteString(fmt.Sprintf("VARCHAR(%d)", col.Length))
		case "decimal":
			def.WriteString(fmt.Sprintf("DECIMAL(%d,%d)", col.Precision, col.Scale))
		case "datetime":
			def.WriteString("TIMESTAMP")
		default:
			def.WriteString(strings.ToUpper(col.Type))
		}
	}

	// Primary key
	if col.Primary && !opts.compositePrimary {
		def.WriteString(" PRIMARY KEY")
	}

	// Not null
	if !col.IsNullable && !col.Primary && !col.AutoIncrement {
		def.WriteString(" NOT NULL")
	}

	// Unique
	if col.IsUnique {
		def.WriteString(" UNIQUE")
	}

	// Default
	if col.DefaultValue != nil {
		switch v := col.DefaultValue.(type) {
		case string:
			def.WriteString(" DEFAULT " + quoteString(v))
		case bool:
			def.WriteString(fmt.Sprintf(" DEFAULT %t", v))
		default:
			def.WriteString(fmt.Sprintf(" DEFAULT %v", v))
		}
	}

	return def.String()
}

// CompileAlter compiles ALTER table commands for PostgreSQL.
func (g *PostgresGrammar) CompileAlter(bp *Blueprint) ([]string, error) {
	var statements []string

	for _, cmd := range bp.commands {
		switch cmd.Type {
		case "add":
			statements = append(statements, g.CompileAddColumn(bp.table, *cmd.Column))
			// ADD COLUMN cannot carry a comment in PostgreSQL; it is its own statement.
			if cmd.Column.ColumnComment != "" {
				statements = append(statements, g.compileComment(bp.table, *cmd.Column))
			}
		case "drop":
			// Generate individual DROP COLUMN statements for better error reporting
			for _, col := range cmd.Columns {
				statements = append(statements, g.CompileDropColumn(bp.table, col))
			}
		case "rename":
			statements = append(statements, g.CompileRenameColumn(bp.table, cmd.OldName, cmd.NewName))
		case "modify":
			// CompileModifyColumn returns multiple statements
			stmts, err := g.CompileModifyColumn(bp.table, *cmd.Column)
			if err != nil {
				return nil, err
			}
			statements = append(statements, stmts...)
		case "dropIndex":
			statements = append(statements, g.CompileDropIndex(bp.table, cmd.Columns))
		case "dropUnique":
			stmts, err := g.CompileDropUnique(bp.table, cmd.Columns)
			if err != nil {
				return nil, err
			}
			statements = append(statements, stmts...)
		case "dropPrimary":
			stmt, err := g.CompileDropPrimary(bp.table)
			if err != nil {
				return nil, err
			}
			statements = append(statements, stmt)
		}
	}

	return statements, nil
}

// CompileAddColumn compiles ADD COLUMN statement for PostgreSQL.
func (g *PostgresGrammar) CompileAddColumn(table string, col ColumnDefinition) string {
	colDef := g.compileColumn(col, columnOptions{})
	return fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s", g.WrapTable(table), colDef)
}

// CompileDropColumn compiles DROP COLUMN statement for PostgreSQL.
func (g *PostgresGrammar) CompileDropColumn(table string, column string) string {
	return fmt.Sprintf("ALTER TABLE %s DROP COLUMN %s", g.WrapTable(table), g.WrapColumn(column))
}

// CompileRenameColumn compiles RENAME COLUMN statement for PostgreSQL.
func (g *PostgresGrammar) CompileRenameColumn(table, from, to string) string {
	return fmt.Sprintf("ALTER TABLE %s RENAME COLUMN %s TO %s",
		g.WrapTable(table), g.WrapColumn(from), g.WrapColumn(to))
}

// CompileModifyColumn compiles ALTER COLUMN statement for PostgreSQL.
func (g *PostgresGrammar) CompileModifyColumn(table string, col ColumnDefinition) ([]string, error) {
	var statements []string
	wrappedTable := g.WrapTable(table)
	wrappedCol := g.WrapColumn(col.Name)

	// Change column type (only if a new type is provided)
	if col.Type != "" {
		var colType string
		switch col.Type {
		case "varchar":
			colType = fmt.Sprintf("VARCHAR(%d)", col.Length)
		case "decimal":
			colType = fmt.Sprintf("DECIMAL(%d,%d)", col.Precision, col.Scale)
		case "datetime":
			colType = "TIMESTAMP"
		default:
			colType = strings.ToUpper(col.Type)
		}
		statements = append(statements, fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s TYPE %s",
			wrappedTable, wrappedCol, colType))
	}

	// Set/drop NOT NULL (only if explicitly set)
	if col.NullableExplicitlySet {
		if col.IsNullable {
			statements = append(statements, fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s DROP NOT NULL",
				wrappedTable, wrappedCol))
		} else {
			statements = append(statements, fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s SET NOT NULL",
				wrappedTable, wrappedCol))
		}
	}

	// Set or drop the default (only if explicitly set). Default(nil) removes the
	// existing default rather than being ignored.
	if col.DefaultExplicitlySet {
		if col.DefaultValue == nil {
			statements = append(statements, fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s DROP DEFAULT",
				wrappedTable, wrappedCol))
		} else {
			var defaultVal string
			switch v := col.DefaultValue.(type) {
			case string:
				defaultVal = quoteString(v)
			case bool:
				defaultVal = fmt.Sprintf("%t", v)
			default:
				defaultVal = fmt.Sprintf("%v", v)
			}
			statements = append(statements, fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s SET DEFAULT %s",
				wrappedTable, wrappedCol, defaultVal))
		}
	}

	if len(statements) == 0 {
		return nil, fmt.Errorf("schema: ModifyColumn(%q) on %q requests no change; set a type, nullability, or default", col.Name, table)
	}

	// Return individual statements, not joined with semicolons
	return statements, nil
}

// CompileDropIndex compiles DROP INDEX statement for PostgreSQL.
func (g *PostgresGrammar) CompileDropIndex(table string, columns []string) string {
	// PostgreSQL uses named indexes, construct index name from table and columns
	indexName := table + "_" + strings.Join(columns, "_") + "_index"
	return fmt.Sprintf("DROP INDEX IF EXISTS %s", g.WrapColumn(indexName))
}

// CompileDropUnique compiles DROP UNIQUE constraint for PostgreSQL.
func (g *PostgresGrammar) CompileDropUnique(table string, columns []string) ([]string, error) {
	// PostgreSQL uses named constraints for unique indexes. When a UNIQUE constraint is created
	// inline without an explicit name, PostgreSQL will by default name it as
	// "<table>_<columns>_key". Older code in this package assumes an explicit name
	// "<table>_<columns>_unique". To support both cases, attempt to drop both names.
	baseName := table + "_" + strings.Join(columns, "_")
	namedConstraint := baseName + "_unique"
	defaultConstraint := baseName + "_key"
	return []string{
		fmt.Sprintf("ALTER TABLE %s DROP CONSTRAINT IF EXISTS %s", g.WrapTable(table), g.WrapColumn(namedConstraint)),
		fmt.Sprintf("ALTER TABLE %s DROP CONSTRAINT IF EXISTS %s", g.WrapTable(table), g.WrapColumn(defaultConstraint)),
	}, nil
}

// CompileDropPrimary compiles DROP PRIMARY KEY for PostgreSQL.
func (g *PostgresGrammar) CompileDropPrimary(table string) (string, error) {
	// PostgreSQL names primary keys as table_pkey by default
	constraintName := table + "_pkey"
	return fmt.Sprintf("ALTER TABLE %s DROP CONSTRAINT IF EXISTS %s", g.WrapTable(table), g.WrapColumn(constraintName)), nil
}

// MySQLGrammar compiles schema for MySQL and MariaDB: backtick-quoted
// identifiers, AUTO_INCREMENT keys, and MODIFY COLUMN alters.
type MySQLGrammar struct {
	// MariaDB marks the grammar as targeting MariaDB rather than MySQL. The two
	// dialects are the same here except where MariaDB accepts syntax MySQL has
	// never had (DROP INDEX IF EXISTS). NewGrammar sets it from the driver name;
	// the zero value is MySQL, which is the stricter of the two.
	MariaDB bool
}

// quoteBacktick wraps a MySQL identifier in backticks, escaping any
// backtick it contains by doubling it.
func quoteBacktick(identifier string) string {
	return "`" + strings.ReplaceAll(identifier, "`", "``") + "`"
}

func (g *MySQLGrammar) WrapTable(table string) string { return quoteBacktick(table) }

func (g *MySQLGrammar) WrapColumn(column string) string { return quoteBacktick(column) }

func (g *MySQLGrammar) CompileTableExists(table string) string {
	return fmt.Sprintf(
		"SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name = %s",
		quoteString(table))
}

func (g *MySQLGrammar) CompileCreate(bp *Blueprint) string {
	var parts []string

	primaryKeys := columnPrimaryKeys(bp, g.WrapColumn)
	opts := columnOptions{compositePrimary: len(primaryKeys) > 1}

	for _, col := range bp.columns {
		parts = append(parts, g.compileColumn(col, opts))
	}

	// A composite key is declared once, as a table constraint; compileColumn
	// left the inline PRIMARY KEY off each of its columns.
	if opts.compositePrimary {
		parts = append(parts, fmt.Sprintf("PRIMARY KEY (%s)", strings.Join(primaryKeys, ", ")))
	}

	parts = append(parts, compileTableConstraints(bp, g.WrapColumn)...)
	parts = append(parts, compileForeignKeys(bp, g.WrapColumn)...)

	return fmt.Sprintf("CREATE TABLE %s (\n  %s\n)", g.WrapTable(bp.table), strings.Join(parts, ",\n  "))
}

// CompileCreateIndexes compiles the CREATE INDEX statements that follow
// a table's creation.
func (g *MySQLGrammar) CompileCreateIndexes(bp *Blueprint) []string {
	return compileCreateIndexStatements(bp, g.WrapTable, g.WrapColumn)
}

// CompileComments returns nothing: MySQL writes a column comment inline in the
// column definition, so compileColumn has already emitted it.
func (g *MySQLGrammar) CompileComments(bp *Blueprint) []string {
	return nil
}

func (g *MySQLGrammar) compileColumn(col ColumnDefinition, opts columnOptions) string {
	var def strings.Builder

	def.WriteString(g.WrapColumn(col.Name))
	def.WriteString(" ")

	switch col.Type {
	case "varchar":
		def.WriteString(fmt.Sprintf("VARCHAR(%d)", col.Length))
	case "decimal":
		def.WriteString(fmt.Sprintf("DECIMAL(%d,%d)", col.Precision, col.Scale))
	case "integer":
		def.WriteString("INT")
	case "bigint":
		def.WriteString("BIGINT")
	case "boolean":
		def.WriteString("TINYINT(1)")
	case "datetime":
		def.WriteString("DATETIME")
	default:
		def.WriteString(strings.ToUpper(col.Type))
	}

	if col.Unsigned {
		def.WriteString(" UNSIGNED")
	}
	if opts.modify {
		// MODIFY COLUMN replaces the whole definition, so every clause left out
		// here is reset to MySQL's default rather than kept. Nullability is
		// therefore only stated when the caller asked for it: a column modified
		// without .Nullable() keeps whatever MySQL infers from the rest of the
		// definition, which for an ordinary column means it becomes nullable.
		// Asserting NOT NULL on a widen-the-type migration would instead fail
		// against the NULLs already stored. There is no fluent NotNullable()
		// setter yet, so only a ColumnDefinition built with NullableExplicitlySet
		// and IsNullable false can ask for NOT NULL on this path.
		if col.NullableExplicitlySet {
			if col.IsNullable {
				def.WriteString(" NULL")
			} else {
				def.WriteString(" NOT NULL")
			}
		}
	} else if !col.IsNullable && !col.Primary {
		// In CREATE TABLE and ADD COLUMN a column is NOT NULL unless declared
		// .Nullable(); there is nothing prior to preserve.
		def.WriteString(" NOT NULL")
	}
	if col.AutoIncrement {
		def.WriteString(" AUTO_INCREMENT")
	}
	if col.Primary && !opts.compositePrimary {
		def.WriteString(" PRIMARY KEY")
	}
	if col.IsUnique {
		def.WriteString(" UNIQUE")
	}
	if col.DefaultValue != nil {
		switch v := col.DefaultValue.(type) {
		case string:
			def.WriteString(" DEFAULT " + quoteString(v))
		case bool:
			if v {
				def.WriteString(" DEFAULT 1")
			} else {
				def.WriteString(" DEFAULT 0")
			}
		default:
			def.WriteString(fmt.Sprintf(" DEFAULT %v", v))
		}
	}
	if col.ColumnComment != "" {
		def.WriteString(" COMMENT " + quoteString(col.ColumnComment))
	}

	return def.String()
}

// CompileAlter compiles ALTER table commands for MySQL.
func (g *MySQLGrammar) CompileAlter(bp *Blueprint) ([]string, error) {
	var statements []string

	for _, cmd := range bp.commands {
		switch cmd.Type {
		case "add":
			statements = append(statements, g.CompileAddColumn(bp.table, *cmd.Column))
		case "drop":
			for _, col := range cmd.Columns {
				statements = append(statements, g.CompileDropColumn(bp.table, col))
			}
		case "rename":
			statements = append(statements, g.CompileRenameColumn(bp.table, cmd.OldName, cmd.NewName))
		case "modify":
			stmts, err := g.CompileModifyColumn(bp.table, *cmd.Column)
			if err != nil {
				return nil, err
			}
			statements = append(statements, stmts...)
		case "dropIndex":
			statements = append(statements, g.CompileDropIndex(bp.table, cmd.Columns))
		case "dropUnique":
			stmts, err := g.CompileDropUnique(bp.table, cmd.Columns)
			if err != nil {
				return nil, err
			}
			statements = append(statements, stmts...)
		case "dropPrimary":
			stmt, err := g.CompileDropPrimary(bp.table)
			if err != nil {
				return nil, err
			}
			statements = append(statements, stmt)
		}
	}

	return statements, nil
}

// CompileAddColumn compiles ADD COLUMN statement for MySQL.
func (g *MySQLGrammar) CompileAddColumn(table string, col ColumnDefinition) string {
	return fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s", g.WrapTable(table), g.compileColumn(col, columnOptions{}))
}

// CompileDropColumn compiles DROP COLUMN statement for MySQL.
func (g *MySQLGrammar) CompileDropColumn(table string, column string) string {
	return fmt.Sprintf("ALTER TABLE %s DROP COLUMN %s", g.WrapTable(table), g.WrapColumn(column))
}

// CompileRenameColumn compiles RENAME COLUMN (MySQL 8+ / MariaDB 10.5+).
func (g *MySQLGrammar) CompileRenameColumn(table, from, to string) string {
	return fmt.Sprintf("ALTER TABLE %s RENAME COLUMN %s TO %s",
		g.WrapTable(table), g.WrapColumn(from), g.WrapColumn(to))
}

// CompileModifyColumn compiles MODIFY COLUMN for MySQL. MySQL replaces
// the whole column definition, so the blueprint must provide the full
// definition including the type. Nullability is only stated when the caller
// set it explicitly — see compileColumn.
func (g *MySQLGrammar) CompileModifyColumn(table string, col ColumnDefinition) ([]string, error) {
	if col.Type == "" {
		return nil, fmt.Errorf("%w: MySQL MODIFY COLUMN replaces the whole definition of %s.%s; specify the column type as well",
			ErrUnsupportedOperation, table, col.Name)
	}
	return []string{
		fmt.Sprintf("ALTER TABLE %s MODIFY COLUMN %s", g.WrapTable(table), g.compileColumn(col, columnOptions{modify: true})),
	}, nil
}

// CompileDropIndex compiles DROP INDEX for MySQL and MariaDB.
//
// The statement is idempotent on MariaDB, which has accepted DROP INDEX IF
// EXISTS since 10.1.4, and is not on MySQL, whose DROP INDEX grammar has no
// IF EXISTS in any release including 8.x and 9.x. Emitting it unconditionally
// would turn every DropIndex migration into a syntax error there, so dropping
// an index that is already gone still fails on MySQL; a migration that needs to
// tolerate it has to guard on information_schema.STATISTICS itself.
func (g *MySQLGrammar) CompileDropIndex(table string, columns []string) string {
	indexName := table + "_" + strings.Join(columns, "_") + "_index"
	if g.MariaDB {
		return fmt.Sprintf("DROP INDEX IF EXISTS %s ON %s", g.WrapColumn(indexName), g.WrapTable(table))
	}
	return fmt.Sprintf("DROP INDEX %s ON %s", g.WrapColumn(indexName), g.WrapTable(table))
}

// CompileDropUnique compiles the drop of a named unique constraint
// ("<table>_<cols>_unique", the name CompileCreate assigns); MySQL
// stores unique constraints as indexes.
func (g *MySQLGrammar) CompileDropUnique(table string, columns []string) ([]string, error) {
	name := table + "_" + strings.Join(columns, "_") + "_unique"
	return []string{
		fmt.Sprintf("ALTER TABLE %s DROP INDEX %s", g.WrapTable(table), g.WrapColumn(name)),
	}, nil
}

// CompileDropPrimary compiles DROP PRIMARY KEY for MySQL.
func (g *MySQLGrammar) CompileDropPrimary(table string) (string, error) {
	return fmt.Sprintf("ALTER TABLE %s DROP PRIMARY KEY", g.WrapTable(table)), nil
}
