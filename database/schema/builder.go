// Package schema provides a fluent schema builder for database migrations.
package schema

import (
	"database/sql"
	"fmt"
	"strings"
)

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

// Create creates a new table.
func (b *Builder) Create(table string, callback func(*Blueprint)) error {
	bp := NewBlueprint(table)
	bp.create = true
	callback(bp)

	sql := b.grammar.CompileCreate(bp)
	_, err := b.db.Exec(sql)
	return err
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
	table   string
	columns []ColumnDefinition
	indexes []IndexDefinition
	create  bool
}

// NewBlueprint creates a new blueprint.
func NewBlueprint(table string) *Blueprint {
	return &Blueprint{
		table:   table,
		columns: make([]ColumnDefinition, 0),
		indexes: make([]IndexDefinition, 0),
	}
}

// ColumnDefinition represents a column definition.
type ColumnDefinition struct {
	Name          string
	Type          string
	Length        int
	Precision     int
	Scale         int
	IsNullable    bool
	DefaultValue  any
	AutoIncrement bool
	Primary       bool
	IsUnique      bool
	IsIndex       bool
	Unsigned      bool
	ColumnComment string
}

// IndexDefinition represents an index definition.
type IndexDefinition struct {
	Name    string
	Columns []string
	Type    string // PRIMARY, UNIQUE, INDEX
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

// Column methods for fluent configuration
func (c *ColumnDefinition) Nullable() *ColumnDefinition {
	c.IsNullable = true
	return c
}

func (c *ColumnDefinition) Default(value any) *ColumnDefinition {
	c.DefaultValue = value
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

// Grammar compiles schema to SQL.
type Grammar interface {
	CompileCreate(bp *Blueprint) string
	CompileTableExists(table string) string
	WrapTable(table string) string
	WrapColumn(column string) string
}

// NewGrammar creates a grammar for the given driver.
func NewGrammar(driver string) Grammar {
	switch driver {
	case "pgsql", "postgres", "postgresql":
		return &PostgresGrammar{}
	default:
		return &SQLiteGrammar{}
	}
}

// SQLiteGrammar compiles schema for SQLite.
type SQLiteGrammar struct{}

func (g *SQLiteGrammar) WrapTable(table string) string {
	return `"` + table + `"`
}

func (g *SQLiteGrammar) WrapColumn(column string) string {
	return `"` + column + `"`
}

func (g *SQLiteGrammar) CompileTableExists(table string) string {
	return fmt.Sprintf("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='%s'", table)
}

func (g *SQLiteGrammar) CompileCreate(bp *Blueprint) string {
	var parts []string
	var primaryKeys []string

	for _, col := range bp.columns {
		def := g.compileColumn(col)
		parts = append(parts, def)
		if col.Primary && !col.AutoIncrement {
			primaryKeys = append(primaryKeys, g.WrapColumn(col.Name))
		}
	}

	// Add composite primary key if needed
	if len(primaryKeys) > 1 {
		parts = append(parts, fmt.Sprintf("PRIMARY KEY (%s)", strings.Join(primaryKeys, ", ")))
	}

	return fmt.Sprintf("CREATE TABLE %s (\n  %s\n)", g.WrapTable(bp.table), strings.Join(parts, ",\n  "))
}

func (g *SQLiteGrammar) compileColumn(col ColumnDefinition) string {
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
	} else if col.Primary {
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
			def.WriteString(fmt.Sprintf(" DEFAULT '%s'", v))
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

// PostgresGrammar compiles schema for PostgreSQL.
type PostgresGrammar struct{}

func (g *PostgresGrammar) WrapTable(table string) string {
	return `"` + table + `"`
}

func (g *PostgresGrammar) WrapColumn(column string) string {
	return `"` + column + `"`
}

func (g *PostgresGrammar) CompileTableExists(table string) string {
	return fmt.Sprintf("SELECT COUNT(*) FROM information_schema.tables WHERE table_name = '%s'", table)
}

func (g *PostgresGrammar) CompileCreate(bp *Blueprint) string {
	var parts []string
	var primaryKeys []string

	for _, col := range bp.columns {
		def := g.compileColumn(col)
		parts = append(parts, def)
		if col.Primary && !col.AutoIncrement {
			primaryKeys = append(primaryKeys, g.WrapColumn(col.Name))
		}
	}

	if len(primaryKeys) > 1 {
		parts = append(parts, fmt.Sprintf("PRIMARY KEY (%s)", strings.Join(primaryKeys, ", ")))
	}

	return fmt.Sprintf("CREATE TABLE %s (\n  %s\n)", g.WrapTable(bp.table), strings.Join(parts, ",\n  "))
}

func (g *PostgresGrammar) compileColumn(col ColumnDefinition) string {
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
	if col.Primary {
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
			def.WriteString(fmt.Sprintf(" DEFAULT '%s'", v))
		case bool:
			def.WriteString(fmt.Sprintf(" DEFAULT %t", v))
		default:
			def.WriteString(fmt.Sprintf(" DEFAULT %v", v))
		}
	}

	return def.String()
}
