package query

import (
	"fmt"
	"sort"
	"strings"

	"github.com/genesysflow/go-genesys/contracts"
)

// NewGrammar creates a new grammar for the given driver.
func NewGrammar(driver string) contracts.Grammar {
	switch driver {
	case "pgsql", "postgres", "postgresql":
		return &PostgresGrammar{}
	case "sqlite", "sqlite3":
		return &SQLiteGrammar{}
	default:
		return &BaseGrammar{}
	}
}

// BaseGrammar provides common SQL compilation functionality.
type BaseGrammar struct {
	paramIndex int
}

// WrapTable wraps a table name with quotes.
func (g *BaseGrammar) WrapTable(table string) string {
	if strings.Contains(table, " ") || strings.Contains(table, ".") {
		return table
	}
	return `"` + table + `"`
}

// WrapColumn wraps a column name with quotes.
func (g *BaseGrammar) WrapColumn(column string) string {
	if column == "*" {
		return column
	}
	if strings.Contains(column, " AS ") || strings.Contains(column, " as ") {
		return column
	}
	if strings.Contains(column, "(") {
		return column
	}
	if strings.Contains(column, ".") {
		parts := strings.Split(column, ".")
		for i, part := range parts {
			if part != "*" {
				parts[i] = `"` + part + `"`
			}
		}
		return strings.Join(parts, ".")
	}
	return `"` + column + `"`
}

// Parameter returns the placeholder for a binding.
func (g *BaseGrammar) Parameter(index int) string {
	return "?"
}

// GetDateFormat returns the database date format.
func (g *BaseGrammar) GetDateFormat() string {
	return "2006-01-02 15:04:05"
}

// CompileSelect compiles a select query.
func (g *BaseGrammar) CompileSelect(builder contracts.QueryBuilder) (string, []any) {
	b := builder.(*Builder)
	g.paramIndex = 0

	parts := make([]string, 0, 8)
	bindings := make([]any, 0)

	// SELECT
	selectClause := "SELECT "
	if b.IsDistinct() {
		selectClause += "DISTINCT "
	}
	columns := make([]string, len(b.GetColumns()))
	for i, col := range b.GetColumns() {
		columns[i] = g.WrapColumn(col)
	}
	selectClause += strings.Join(columns, ", ")
	parts = append(parts, selectClause)

	// FROM
	parts = append(parts, "FROM "+g.WrapTable(b.GetTable()))

	// JOINS
	for _, join := range b.GetJoins() {
		if join.joinType == "CROSS" {
			parts = append(parts, fmt.Sprintf("CROSS JOIN %s", g.WrapTable(join.table)))
		} else {
			parts = append(parts, fmt.Sprintf(
				"%s JOIN %s ON %s %s %s",
				join.joinType,
				g.WrapTable(join.table),
				g.WrapColumn(join.first),
				join.operator,
				g.WrapColumn(join.second),
			))
		}
	}

	// WHERE
	whereParts, whereBindings := g.compileWheres(b.GetWheres())
	if len(whereParts) > 0 {
		parts = append(parts, "WHERE "+whereParts)
		bindings = append(bindings, whereBindings...)
	}

	// GROUP BY
	if len(b.GetGroups()) > 0 {
		groupCols := make([]string, len(b.GetGroups()))
		for i, col := range b.GetGroups() {
			groupCols[i] = g.WrapColumn(col)
		}
		parts = append(parts, "GROUP BY "+strings.Join(groupCols, ", "))
	}

	// HAVING
	havingParts, havingBindings := g.compileHavings(b.GetHavings())
	if len(havingParts) > 0 {
		parts = append(parts, "HAVING "+havingParts)
		bindings = append(bindings, havingBindings...)
	}

	// ORDER BY
	if len(b.GetOrders()) > 0 {
		orderParts := make([]string, 0, len(b.GetOrders()))
		for _, order := range b.GetOrders() {
			if order.rawSQL != "" {
				orderParts = append(orderParts, order.rawSQL)
			} else {
				orderParts = append(orderParts, fmt.Sprintf("%s %s", g.WrapColumn(order.column), order.direction))
			}
		}
		parts = append(parts, "ORDER BY "+strings.Join(orderParts, ", "))
	}

	// LIMIT
	if b.GetLimit() != nil {
		parts = append(parts, fmt.Sprintf("LIMIT %d", *b.GetLimit()))
	}

	// OFFSET
	if b.GetOffset() != nil {
		parts = append(parts, fmt.Sprintf("OFFSET %d", *b.GetOffset()))
	}

	return strings.Join(parts, " "), bindings
}

func (g *BaseGrammar) compileWheres(wheres []whereClause) (string, []any) {
	if len(wheres) == 0 {
		return "", nil
	}

	parts := make([]string, 0, len(wheres))
	bindings := make([]any, 0)

	for i, w := range wheres {
		var wherePart string

		switch w.whereType {
		case "basic":
			wherePart = fmt.Sprintf("%s %s %s", g.WrapColumn(w.column), w.operator, g.Parameter(g.paramIndex))
			g.paramIndex++
			bindings = append(bindings, w.value)

		case "in":
			placeholders := make([]string, len(w.values))
			for j := range w.values {
				placeholders[j] = g.Parameter(g.paramIndex)
				g.paramIndex++
			}
			wherePart = fmt.Sprintf("%s IN (%s)", g.WrapColumn(w.column), strings.Join(placeholders, ", "))
			bindings = append(bindings, w.values...)

		case "not_in":
			placeholders := make([]string, len(w.values))
			for j := range w.values {
				placeholders[j] = g.Parameter(g.paramIndex)
				g.paramIndex++
			}
			wherePart = fmt.Sprintf("%s NOT IN (%s)", g.WrapColumn(w.column), strings.Join(placeholders, ", "))
			bindings = append(bindings, w.values...)

		case "null":
			wherePart = fmt.Sprintf("%s IS NULL", g.WrapColumn(w.column))

		case "not_null":
			wherePart = fmt.Sprintf("%s IS NOT NULL", g.WrapColumn(w.column))

		case "between":
			wherePart = fmt.Sprintf("%s BETWEEN %s AND %s",
				g.WrapColumn(w.column), g.Parameter(g.paramIndex), g.Parameter(g.paramIndex+1))
			g.paramIndex += 2
			bindings = append(bindings, w.low, w.high)

		case "raw":
			wherePart = w.rawSQL
			bindings = append(bindings, w.rawBinding...)
		}

		if i > 0 {
			parts = append(parts, w.boolean+" "+wherePart)
		} else {
			parts = append(parts, wherePart)
		}
	}

	return strings.Join(parts, " "), bindings
}

func (g *BaseGrammar) compileHavings(havings []havingClause) (string, []any) {
	if len(havings) == 0 {
		return "", nil
	}

	parts := make([]string, 0, len(havings))
	bindings := make([]any, 0)

	for i, h := range havings {
		var havingPart string
		if h.rawSQL != "" {
			havingPart = h.rawSQL
			bindings = append(bindings, h.rawBinding...)
		} else {
			havingPart = fmt.Sprintf("%s %s %s", g.WrapColumn(h.column), h.operator, g.Parameter(g.paramIndex))
			g.paramIndex++
			bindings = append(bindings, h.value)
		}

		if i > 0 {
			parts = append(parts, "AND "+havingPart)
		} else {
			parts = append(parts, havingPart)
		}
	}

	return strings.Join(parts, " "), bindings
}

// CompileInsert compiles an insert query.
func (g *BaseGrammar) CompileInsert(builder contracts.QueryBuilder, values map[string]any) (string, []any) {
	b := builder.(*Builder)
	g.paramIndex = 0

	// Sort keys for consistent ordering
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	columns := make([]string, len(keys))
	placeholders := make([]string, len(keys))
	bindings := make([]any, len(keys))

	for i, key := range keys {
		columns[i] = g.WrapColumn(key)
		placeholders[i] = g.Parameter(g.paramIndex)
		g.paramIndex++
		bindings[i] = values[key]
	}

	sql := fmt.Sprintf(
		"INSERT INTO %s (%s) VALUES (%s)",
		g.WrapTable(b.GetTable()),
		strings.Join(columns, ", "),
		strings.Join(placeholders, ", "),
	)

	return sql, bindings
}

// CompileUpdate compiles an update query.
func (g *BaseGrammar) CompileUpdate(builder contracts.QueryBuilder, values map[string]any) (string, []any) {
	b := builder.(*Builder)
	g.paramIndex = 0

	// Sort keys for consistent ordering
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	setParts := make([]string, len(keys))
	bindings := make([]any, 0, len(keys))

	for i, key := range keys {
		val := values[key]
		if raw, ok := val.(RawExpression); ok {
			setParts[i] = fmt.Sprintf("%s = %s", g.WrapColumn(key), string(raw))
		} else {
			setParts[i] = fmt.Sprintf("%s = %s", g.WrapColumn(key), g.Parameter(g.paramIndex))
			g.paramIndex++
			bindings = append(bindings, val)
		}
	}

	sql := fmt.Sprintf("UPDATE %s SET %s", g.WrapTable(b.GetTable()), strings.Join(setParts, ", "))

	// WHERE
	whereParts, whereBindings := g.compileWheres(b.GetWheres())
	if len(whereParts) > 0 {
		sql += " WHERE " + whereParts
		bindings = append(bindings, whereBindings...)
	}

	return sql, bindings
}

// CompileDelete compiles a delete query.
func (g *BaseGrammar) CompileDelete(builder contracts.QueryBuilder) (string, []any) {
	b := builder.(*Builder)
	g.paramIndex = 0

	sql := fmt.Sprintf("DELETE FROM %s", g.WrapTable(b.GetTable()))

	// WHERE
	whereParts, whereBindings := g.compileWheres(b.GetWheres())
	if len(whereParts) > 0 {
		sql += " WHERE " + whereParts
	}

	return sql, whereBindings
}

// CompileExists compiles an exists query.
func (g *BaseGrammar) CompileExists(builder contracts.QueryBuilder) (string, []any) {
	selectSQL, bindings := g.CompileSelect(builder)
	return fmt.Sprintf("SELECT EXISTS (%s) AS \"exists\"", selectSQL), bindings
}

// PostgresGrammar is the grammar for PostgreSQL.
type PostgresGrammar struct {
	BaseGrammar
}

// Parameter returns PostgreSQL-style numbered placeholders.
func (g *PostgresGrammar) Parameter(index int) string {
	return fmt.Sprintf("$%d", index+1)
}

// SQLiteGrammar is the grammar for SQLite.
type SQLiteGrammar struct {
	BaseGrammar
}

// WrapTable for SQLite uses different quoting.
func (g *SQLiteGrammar) WrapTable(table string) string {
	if strings.Contains(table, " ") || strings.Contains(table, ".") {
		return table
	}
	return `"` + table + `"`
}

// WrapColumn for SQLite uses different quoting.
func (g *SQLiteGrammar) WrapColumn(column string) string {
	if column == "*" {
		return column
	}
	if strings.Contains(column, " AS ") || strings.Contains(column, " as ") {
		return column
	}
	if strings.Contains(column, "(") {
		return column
	}
	if strings.Contains(column, ".") {
		parts := strings.Split(column, ".")
		for i, part := range parts {
			if part != "*" {
				parts[i] = `"` + part + `"`
			}
		}
		return strings.Join(parts, ".")
	}
	return `"` + column + `"`
}
