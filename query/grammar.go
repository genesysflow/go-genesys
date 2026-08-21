// Package query provides a fluent, Laravel-style SQL query builder.
package query

import (
	"fmt"
	"strings"
)

// Grammar compiles builder state into SQL for a specific database driver.
type Grammar struct {
	driver string
}

// NewGrammar creates a grammar for the given driver name
// (pgsql/postgres/postgresql, mysql, sqlite/sqlite3).
func NewGrammar(driver string) *Grammar {
	if driver == "mariadb" {
		driver = "mysql" // MariaDB speaks the MySQL dialect
	}
	return &Grammar{driver: driver}
}

// Driver returns the driver name the grammar compiles for.
func (g *Grammar) Driver() string {
	return g.driver
}

// isPostgres reports whether the driver uses $N placeholders.
func (g *Grammar) isPostgres() bool {
	switch g.driver {
	case "pgsql", "postgres", "postgresql":
		return true
	}
	return false
}

// Placeholder returns the bind placeholder for the given 1-based position.
func (g *Grammar) Placeholder(position int) string {
	if g.isPostgres() {
		return fmt.Sprintf("$%d", position)
	}
	return "?"
}

// Parameterize renders count placeholders starting at the given 1-based position.
func (g *Grammar) Parameterize(start, count int) string {
	parts := make([]string, count)
	for i := 0; i < count; i++ {
		parts[i] = g.Placeholder(start + i)
	}
	return strings.Join(parts, ", ")
}

// Wrap quotes an identifier, honouring dots ("users.name") and aliases ("u.name as author").
func (g *Grammar) Wrap(identifier string) string {
	lower := strings.ToLower(identifier)
	if idx := strings.Index(lower, " as "); idx >= 0 {
		return g.Wrap(identifier[:idx]) + " AS " + g.wrapSegment(identifier[idx+4:])
	}
	parts := strings.Split(identifier, ".")
	for i, part := range parts {
		parts[i] = g.wrapSegment(part)
	}
	return strings.Join(parts, ".")
}

func (g *Grammar) wrapSegment(segment string) string {
	segment = strings.TrimSpace(segment)
	if segment == "*" {
		return "*"
	}
	if g.driver == "mysql" {
		return "`" + strings.ReplaceAll(segment, "`", "``") + "`"
	}
	return `"` + strings.ReplaceAll(segment, `"`, `""`) + `"`
}

// WrapTable quotes a table name.
func (g *Grammar) WrapTable(table string) string {
	return g.Wrap(table)
}

// CompileSelect compiles the builder into a SELECT statement and its bindings.
func (g *Grammar) CompileSelect(b *Builder) (string, []any) {
	var sqlParts []string
	var bindings []any

	columns := "*"
	if len(b.columns) > 0 {
		wrapped := make([]string, len(b.columns))
		for i, c := range b.columns {
			wrapped[i] = g.wrapColumnExpression(c)
		}
		columns = strings.Join(wrapped, ", ")
	}

	sel := "SELECT "
	if b.distinct {
		sel += "DISTINCT "
	}
	sqlParts = append(sqlParts, sel+columns)
	sqlParts = append(sqlParts, "FROM "+g.WrapTable(b.table))

	for _, j := range b.joins {
		if j.kind == "CROSS" {
			sqlParts = append(sqlParts, "CROSS JOIN "+g.WrapTable(j.table))
			continue
		}
		sqlParts = append(sqlParts, fmt.Sprintf("%s JOIN %s ON %s %s %s",
			j.kind, g.WrapTable(j.table), g.Wrap(j.first), j.operator, g.Wrap(j.second)))
	}

	if clause, whereBindings := g.compileWheres(b.wheres, len(bindings)); clause != "" {
		sqlParts = append(sqlParts, "WHERE "+clause)
		bindings = append(bindings, whereBindings...)
	}

	if len(b.groups) > 0 {
		wrapped := make([]string, len(b.groups))
		for i, c := range b.groups {
			wrapped[i] = g.Wrap(c)
		}
		sqlParts = append(sqlParts, "GROUP BY "+strings.Join(wrapped, ", "))
	}

	if clause, havingBindings := g.compileWheres(b.havings, len(bindings)); clause != "" {
		sqlParts = append(sqlParts, "HAVING "+clause)
		bindings = append(bindings, havingBindings...)
	}

	// Unions come before ORDER BY / LIMIT, which then apply to the
	// combined result (standard SQL and Laravel semantics).
	for _, u := range b.unions {
		unionSQL, unionBindings := u.builder.grammar.CompileSelect(u.builder)
		renumbered := g.renumberPlaceholders(neutralizePlaceholders(unionSQL), len(bindings))
		keyword := "UNION"
		if u.all {
			keyword = "UNION ALL"
		}
		sqlParts = append(sqlParts, keyword+" "+renumbered)
		bindings = append(bindings, unionBindings...)
	}

	if len(b.orders) > 0 {
		parts := make([]string, len(b.orders))
		for i, o := range b.orders {
			if o.raw != "" {
				parts[i] = o.raw
				continue
			}
			direction := "ASC"
			if strings.EqualFold(o.direction, "desc") {
				direction = "DESC"
			}
			parts[i] = g.Wrap(o.column) + " " + direction
		}
		sqlParts = append(sqlParts, "ORDER BY "+strings.Join(parts, ", "))
	}

	if b.limit >= 0 {
		sqlParts = append(sqlParts, fmt.Sprintf("LIMIT %d", b.limit))
	} else if b.offset > 0 && !g.isPostgres() {
		// SQLite and MySQL reject OFFSET without LIMIT; emit the
		// driver's "no limit" sentinel so Skip() alone still works.
		if g.driver == "mysql" {
			sqlParts = append(sqlParts, "LIMIT 18446744073709551615")
		} else {
			sqlParts = append(sqlParts, "LIMIT -1")
		}
	}
	if b.offset > 0 {
		sqlParts = append(sqlParts, fmt.Sprintf("OFFSET %d", b.offset))
	}

	if lock := g.compileLock(b.lock); lock != "" {
		sqlParts = append(sqlParts, lock)
	}

	return strings.Join(sqlParts, " "), bindings
}

// wrapColumnExpression wraps a column unless it is a raw expression or contains a function call.
func (g *Grammar) wrapColumnExpression(column string) string {
	if strings.ContainsAny(column, "()") {
		return column
	}
	return g.Wrap(column)
}

// compileWheres compiles a list of where clauses; offset is the number of
// bindings already consumed (for positional placeholders).
func (g *Grammar) compileWheres(wheres []where, offset int) (string, []any) {
	if len(wheres) == 0 {
		return "", nil
	}

	var sb strings.Builder
	var bindings []any
	position := offset

	for i, w := range wheres {
		if i > 0 {
			sb.WriteString(" " + w.boolean + " ")
		}

		switch w.kind {
		case whereBasic:
			sb.WriteString(g.Wrap(w.column) + " " + w.operator + " " + g.Placeholder(position+1))
			bindings = append(bindings, w.value)
			position++
		case whereColumn:
			sb.WriteString(g.Wrap(w.column) + " " + w.operator + " " + g.Wrap(w.valueColumn))
		case whereIn, whereNotIn:
			op := "IN"
			if w.kind == whereNotIn {
				op = "NOT IN"
			}
			if len(w.values) == 0 {
				// An empty IN list matches nothing; NOT IN matches everything.
				if w.kind == whereIn {
					sb.WriteString("1 = 0")
				} else {
					sb.WriteString("1 = 1")
				}
				continue
			}
			sb.WriteString(g.Wrap(w.column) + " " + op + " (" + g.Parameterize(position+1, len(w.values)) + ")")
			bindings = append(bindings, w.values...)
			position += len(w.values)
		case whereNull:
			sb.WriteString(g.Wrap(w.column) + " IS NULL")
		case whereNotNull:
			sb.WriteString(g.Wrap(w.column) + " IS NOT NULL")
		case whereBetween:
			sb.WriteString(g.Wrap(w.column) + " BETWEEN " + g.Placeholder(position+1) + " AND " + g.Placeholder(position+2))
			bindings = append(bindings, w.values[0], w.values[1])
			position += 2
		case whereRaw:
			raw := w.rawSQL
			if g.isPostgres() {
				raw = g.renumberPlaceholders(raw, position)
			}
			sb.WriteString(raw)
			bindings = append(bindings, w.values...)
			position += len(w.values)
		case whereNested:
			clause, nested := g.compileWheres(w.nested, position)
			sb.WriteString("(" + clause + ")")
			bindings = append(bindings, nested...)
			position += len(nested)
		}
	}

	return sb.String(), bindings
}

// renumberPlaceholders rewrites ? placeholders in raw SQL to $N for postgres.
func (g *Grammar) renumberPlaceholders(raw string, offset int) string {
	var sb strings.Builder
	n := offset
	for _, r := range raw {
		if r == '?' {
			n++
			sb.WriteString(fmt.Sprintf("$%d", n))
			continue
		}
		sb.WriteRune(r)
	}
	return sb.String()
}

// CompileInsert compiles an INSERT for one or more rows sharing the same columns.
func (g *Grammar) CompileInsert(table string, columns []string, rows int) string {
	wrapped := make([]string, len(columns))
	for i, c := range columns {
		wrapped[i] = g.Wrap(c)
	}

	var valueGroups []string
	position := 0
	for r := 0; r < rows; r++ {
		valueGroups = append(valueGroups, "("+g.Parameterize(position+1, len(columns))+")")
		position += len(columns)
	}

	return fmt.Sprintf("INSERT INTO %s (%s) VALUES %s",
		g.WrapTable(table), strings.Join(wrapped, ", "), strings.Join(valueGroups, ", "))
}

// CompileUpdate compiles an UPDATE statement; where bindings follow set bindings.
func (g *Grammar) CompileUpdate(b *Builder, columns []string) (string, []any) {
	sets := make([]string, len(columns))
	for i, c := range columns {
		sets[i] = g.Wrap(c) + " = " + g.Placeholder(i+1)
	}

	sqlStr := fmt.Sprintf("UPDATE %s SET %s", g.WrapTable(b.table), strings.Join(sets, ", "))

	clause, bindings := g.compileWheres(b.wheres, len(columns))
	if clause != "" {
		sqlStr += " WHERE " + clause
	}
	return sqlStr, bindings
}

// CompileDelete compiles a DELETE statement.
func (g *Grammar) CompileDelete(b *Builder) (string, []any) {
	sqlStr := "DELETE FROM " + g.WrapTable(b.table)
	clause, bindings := g.compileWheres(b.wheres, 0)
	if clause != "" {
		sqlStr += " WHERE " + clause
	}
	return sqlStr, bindings
}
