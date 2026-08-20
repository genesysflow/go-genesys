package query

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// ErrNoRows is returned by First and Value when no record matches the query.
var ErrNoRows = sql.ErrNoRows

// Executor is the minimal database surface the builder needs.
// Both contracts.Connection and contracts.Transaction satisfy it.
type Executor interface {
	Query(query string, bindings ...any) (*sql.Rows, error)
	QueryRow(query string, bindings ...any) *sql.Row
	Exec(query string, bindings ...any) (sql.Result, error)
}

type whereKind int

const (
	whereBasic whereKind = iota
	whereColumn
	whereIn
	whereNotIn
	whereNull
	whereNotNull
	whereBetween
	whereRaw
	whereNested
)

type where struct {
	kind        whereKind
	boolean     string // AND / OR
	column      string
	operator    string
	value       any
	valueColumn string
	values      []any
	rawSQL      string
	nested      []where
}

type join struct {
	kind     string // INNER / LEFT / RIGHT / CROSS
	table    string
	first    string
	operator string
	second   string
}

type order struct {
	column    string
	direction string
	raw       string
}

// Builder is a fluent SQL query builder.
type Builder struct {
	executor Executor
	grammar  *Grammar

	table    string
	columns  []string
	distinct bool
	joins    []join
	wheres   []where
	groups   []string
	havings  []where
	orders   []order
	limit    int
	offset   int
}

// New creates a builder for the given driver and executor.
func New(driver string, executor Executor) *Builder {
	return &Builder{
		executor: executor,
		grammar:  NewGrammar(driver),
		limit:    -1,
	}
}

// Table sets the table the query targets.
func (b *Builder) Table(table string) *Builder {
	b.table = table
	return b
}

// Clone returns a deep copy of the builder.
func (b *Builder) Clone() *Builder {
	clone := *b
	clone.columns = append([]string(nil), b.columns...)
	clone.joins = append([]join(nil), b.joins...)
	clone.wheres = append([]where(nil), b.wheres...)
	clone.groups = append([]string(nil), b.groups...)
	clone.havings = append([]where(nil), b.havings...)
	clone.orders = append([]order(nil), b.orders...)
	return &clone
}

// Select sets the columns to retrieve.
func (b *Builder) Select(columns ...string) *Builder {
	b.columns = columns
	return b
}

// AddSelect appends columns to the select list.
func (b *Builder) AddSelect(columns ...string) *Builder {
	b.columns = append(b.columns, columns...)
	return b
}

// Distinct marks the query as SELECT DISTINCT.
func (b *Builder) Distinct() *Builder {
	b.distinct = true
	return b
}

// Where adds a basic where clause. Accepts (column, value) or (column, operator, value).
func (b *Builder) Where(column string, args ...any) *Builder {
	return b.addWhere("AND", column, args...)
}

// OrWhere adds a basic where clause joined with OR.
func (b *Builder) OrWhere(column string, args ...any) *Builder {
	return b.addWhere("OR", column, args...)
}

func (b *Builder) addWhere(boolean, column string, args ...any) *Builder {
	operator := "="
	var value any
	switch len(args) {
	case 1:
		value = args[0]
	case 2:
		operator = fmt.Sprint(args[0])
		value = args[1]
	default:
		panic("query: Where expects (column, value) or (column, operator, value)")
	}
	if value == nil {
		if operator == "!=" || operator == "<>" {
			b.wheres = append(b.wheres, where{kind: whereNotNull, boolean: boolean, column: column})
		} else {
			b.wheres = append(b.wheres, where{kind: whereNull, boolean: boolean, column: column})
		}
		return b
	}
	b.wheres = append(b.wheres, where{kind: whereBasic, boolean: boolean, column: column, operator: operator, value: value})
	return b
}

// WhereColumn compares two columns.
func (b *Builder) WhereColumn(first, operator, second string) *Builder {
	b.wheres = append(b.wheres, where{kind: whereColumn, boolean: "AND", column: first, operator: operator, valueColumn: second})
	return b
}

// WhereIn adds a WHERE column IN (...) clause.
func (b *Builder) WhereIn(column string, values ...any) *Builder {
	b.wheres = append(b.wheres, where{kind: whereIn, boolean: "AND", column: column, values: flatten(values)})
	return b
}

// WhereNotIn adds a WHERE column NOT IN (...) clause.
func (b *Builder) WhereNotIn(column string, values ...any) *Builder {
	b.wheres = append(b.wheres, where{kind: whereNotIn, boolean: "AND", column: column, values: flatten(values)})
	return b
}

// OrWhereIn adds an OR column IN (...) clause.
func (b *Builder) OrWhereIn(column string, values ...any) *Builder {
	b.wheres = append(b.wheres, where{kind: whereIn, boolean: "OR", column: column, values: flatten(values)})
	return b
}

// WhereNull adds a WHERE column IS NULL clause.
func (b *Builder) WhereNull(column string) *Builder {
	b.wheres = append(b.wheres, where{kind: whereNull, boolean: "AND", column: column})
	return b
}

// WhereNotNull adds a WHERE column IS NOT NULL clause.
func (b *Builder) WhereNotNull(column string) *Builder {
	b.wheres = append(b.wheres, where{kind: whereNotNull, boolean: "AND", column: column})
	return b
}

// WhereBetween adds a WHERE column BETWEEN low AND high clause.
func (b *Builder) WhereBetween(column string, low, high any) *Builder {
	b.wheres = append(b.wheres, where{kind: whereBetween, boolean: "AND", column: column, values: []any{low, high}})
	return b
}

// WhereRaw adds a raw where clause with ? placeholders.
func (b *Builder) WhereRaw(rawSQL string, bindings ...any) *Builder {
	b.wheres = append(b.wheres, where{kind: whereRaw, boolean: "AND", rawSQL: rawSQL, values: bindings})
	return b
}

// WhereGroup adds a nested (grouped) where clause: WHERE (...).
func (b *Builder) WhereGroup(fn func(*Builder)) *Builder {
	return b.whereGroup("AND", fn)
}

// OrWhereGroup adds a nested where clause joined with OR.
func (b *Builder) OrWhereGroup(fn func(*Builder)) *Builder {
	return b.whereGroup("OR", fn)
}

func (b *Builder) whereGroup(boolean string, fn func(*Builder)) *Builder {
	nested := New(b.grammar.Driver(), b.executor)
	fn(nested)
	if len(nested.wheres) > 0 {
		b.wheres = append(b.wheres, where{kind: whereNested, boolean: boolean, nested: nested.wheres})
	}
	return b
}

// Join adds an INNER JOIN clause.
func (b *Builder) Join(table, first, operator, second string) *Builder {
	b.joins = append(b.joins, join{kind: "INNER", table: table, first: first, operator: operator, second: second})
	return b
}

// LeftJoin adds a LEFT JOIN clause.
func (b *Builder) LeftJoin(table, first, operator, second string) *Builder {
	b.joins = append(b.joins, join{kind: "LEFT", table: table, first: first, operator: operator, second: second})
	return b
}

// RightJoin adds a RIGHT JOIN clause.
func (b *Builder) RightJoin(table, first, operator, second string) *Builder {
	b.joins = append(b.joins, join{kind: "RIGHT", table: table, first: first, operator: operator, second: second})
	return b
}

// GroupBy adds GROUP BY columns.
func (b *Builder) GroupBy(columns ...string) *Builder {
	b.groups = append(b.groups, columns...)
	return b
}

// Having adds a HAVING clause. Accepts (column, value) or (column, operator, value).
func (b *Builder) Having(column string, args ...any) *Builder {
	operator := "="
	var value any
	switch len(args) {
	case 1:
		value = args[0]
	case 2:
		operator = fmt.Sprint(args[0])
		value = args[1]
	default:
		panic("query: Having expects (column, value) or (column, operator, value)")
	}
	b.havings = append(b.havings, where{kind: whereBasic, boolean: "AND", column: column, operator: operator, value: value})
	return b
}

// HavingRaw adds a raw HAVING clause with ? placeholders.
func (b *Builder) HavingRaw(rawSQL string, bindings ...any) *Builder {
	b.havings = append(b.havings, where{kind: whereRaw, boolean: "AND", rawSQL: rawSQL, values: bindings})
	return b
}

// OrderBy adds an ORDER BY clause (direction defaults to asc).
func (b *Builder) OrderBy(column string, direction ...string) *Builder {
	dir := "asc"
	if len(direction) > 0 {
		dir = direction[0]
	}
	b.orders = append(b.orders, order{column: column, direction: dir})
	return b
}

// OrderByDesc adds a descending ORDER BY clause.
func (b *Builder) OrderByDesc(column string) *Builder {
	return b.OrderBy(column, "desc")
}

// OrderByRaw adds a raw ORDER BY expression.
func (b *Builder) OrderByRaw(rawSQL string) *Builder {
	b.orders = append(b.orders, order{raw: rawSQL})
	return b
}

// Latest orders by the given column (default created_at) descending.
func (b *Builder) Latest(column ...string) *Builder {
	col := "created_at"
	if len(column) > 0 {
		col = column[0]
	}
	return b.OrderByDesc(col)
}

// Oldest orders by the given column (default created_at) ascending.
func (b *Builder) Oldest(column ...string) *Builder {
	col := "created_at"
	if len(column) > 0 {
		col = column[0]
	}
	return b.OrderBy(col)
}

// Limit sets the maximum number of rows to return.
func (b *Builder) Limit(limit int) *Builder {
	b.limit = limit
	return b
}

// Take is an alias for Limit.
func (b *Builder) Take(limit int) *Builder {
	return b.Limit(limit)
}

// Offset sets the number of rows to skip.
func (b *Builder) Offset(offset int) *Builder {
	b.offset = offset
	return b
}

// Skip is an alias for Offset.
func (b *Builder) Skip(offset int) *Builder {
	return b.Offset(offset)
}

// When applies the callback if the condition is true.
func (b *Builder) When(condition bool, fn func(*Builder)) *Builder {
	if condition {
		fn(b)
	}
	return b
}

// ToSQL returns the compiled SELECT statement and its bindings without executing.
func (b *Builder) ToSQL() (string, []any) {
	return b.grammar.CompileSelect(b)
}

// Rows executes the query and returns the raw *sql.Rows for custom scanning.
func (b *Builder) Rows() (*sql.Rows, error) {
	sqlStr, bindings := b.ToSQL()
	return b.executor.Query(sqlStr, bindings...)
}

// Get executes the query and returns all rows as maps.
func (b *Builder) Get() ([]map[string]any, error) {
	sqlStr, bindings := b.ToSQL()
	rows, err := b.executor.Query(sqlStr, bindings...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return ScanRows(rows)
}

// First returns the first matching row, or ErrNoRows when none matches.
func (b *Builder) First() (map[string]any, error) {
	results, err := b.Clone().Limit(1).Get()
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, ErrNoRows
	}
	return results[0], nil
}

// Find returns the row with the given id (primary key column defaults to "id").
func (b *Builder) Find(id any, column ...string) (map[string]any, error) {
	pk := "id"
	if len(column) > 0 {
		pk = column[0]
	}
	return b.Clone().Where(pk, id).First()
}

// Exists reports whether any row matches the query.
func (b *Builder) Exists() (bool, error) {
	count, err := b.Count()
	return count > 0, err
}

// Value returns a single column's value from the first matching row.
func (b *Builder) Value(column string) (any, error) {
	row, err := b.Clone().Select(column).First()
	if err != nil {
		return nil, err
	}
	return row[columnAlias(column)], nil
}

// Pluck returns a single column's values from all matching rows.
func (b *Builder) Pluck(column string) ([]any, error) {
	rows, err := b.Clone().Select(column).Get()
	if err != nil {
		return nil, err
	}
	key := columnAlias(column)
	values := make([]any, 0, len(rows))
	for _, row := range rows {
		values = append(values, row[key])
	}
	return values, nil
}

// columnAlias returns the key a selected column appears under in result maps.
func columnAlias(column string) string {
	lower := strings.ToLower(column)
	if idx := strings.Index(lower, " as "); idx >= 0 {
		return strings.TrimSpace(column[idx+4:])
	}
	if idx := strings.LastIndex(column, "."); idx >= 0 {
		return column[idx+1:]
	}
	return column
}

// Count returns the number of matching rows.
func (b *Builder) Count(column ...string) (int64, error) {
	col := "*"
	if len(column) > 0 {
		col = b.grammar.Wrap(column[0])
	}
	value, err := b.aggregate(fmt.Sprintf("COUNT(%s)", col))
	if err != nil {
		return 0, err
	}
	return toInt64(value), nil
}

// Sum returns the sum of a column.
func (b *Builder) Sum(column string) (float64, error) {
	value, err := b.aggregate(fmt.Sprintf("SUM(%s)", b.grammar.Wrap(column)))
	if err != nil {
		return 0, err
	}
	return toFloat64(value), nil
}

// Avg returns the average of a column.
func (b *Builder) Avg(column string) (float64, error) {
	value, err := b.aggregate(fmt.Sprintf("AVG(%s)", b.grammar.Wrap(column)))
	if err != nil {
		return 0, err
	}
	return toFloat64(value), nil
}

// Max returns the maximum value of a column.
func (b *Builder) Max(column string) (any, error) {
	return b.aggregate(fmt.Sprintf("MAX(%s)", b.grammar.Wrap(column)))
}

// Min returns the minimum value of a column.
func (b *Builder) Min(column string) (any, error) {
	return b.aggregate(fmt.Sprintf("MIN(%s)", b.grammar.Wrap(column)))
}

func (b *Builder) aggregate(expression string) (any, error) {
	clone := b.Clone()
	clone.columns = []string{expression + " AS aggregate"}
	clone.orders = nil
	clone.limit = -1
	clone.offset = 0
	sqlStr, bindings := clone.grammar.CompileSelect(clone)

	var value any
	if err := b.executor.QueryRow(sqlStr, bindings...).Scan(&value); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return value, nil
}

// Insert inserts one or more rows. All rows must share the same columns.
func (b *Builder) Insert(rows ...map[string]any) error {
	if len(rows) == 0 {
		return nil
	}
	columns := sortedKeys(rows[0])
	sqlStr := b.grammar.CompileInsert(b.table, columns, len(rows))

	var bindings []any
	for _, row := range rows {
		for _, c := range columns {
			bindings = append(bindings, row[c])
		}
	}
	_, err := b.executor.Exec(sqlStr, bindings...)
	return err
}

// InsertGetID inserts a row and returns its auto-increment id.
func (b *Builder) InsertGetID(values map[string]any, idColumn ...string) (int64, error) {
	columns := sortedKeys(values)
	bindings := make([]any, 0, len(columns))
	for _, c := range columns {
		bindings = append(bindings, values[c])
	}

	sqlStr := b.grammar.CompileInsert(b.table, columns, 1)
	if b.grammar.isPostgres() {
		pk := "id"
		if len(idColumn) > 0 {
			pk = idColumn[0]
		}
		sqlStr += " RETURNING " + b.grammar.Wrap(pk)
		var id int64
		if err := b.executor.QueryRow(sqlStr, bindings...).Scan(&id); err != nil {
			return 0, err
		}
		return id, nil
	}

	result, err := b.executor.Exec(sqlStr, bindings...)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

// Update updates matching rows and returns the number of affected rows.
func (b *Builder) Update(values map[string]any) (int64, error) {
	columns := sortedKeys(values)
	sqlStr, whereBindings := b.grammar.CompileUpdate(b, columns)

	bindings := make([]any, 0, len(columns)+len(whereBindings))
	for _, c := range columns {
		bindings = append(bindings, values[c])
	}
	bindings = append(bindings, whereBindings...)

	result, err := b.executor.Exec(sqlStr, bindings...)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// Increment increases a column by the given amount (default 1).
func (b *Builder) Increment(column string, amount ...int) (int64, error) {
	return b.incrementBy(column, "+", amount...)
}

// Decrement decreases a column by the given amount (default 1).
func (b *Builder) Decrement(column string, amount ...int) (int64, error) {
	return b.incrementBy(column, "-", amount...)
}

func (b *Builder) incrementBy(column, op string, amount ...int) (int64, error) {
	n := 1
	if len(amount) > 0 {
		n = amount[0]
	}
	wrapped := b.grammar.Wrap(column)
	sqlStr := fmt.Sprintf("UPDATE %s SET %s = %s %s %d", b.grammar.WrapTable(b.table), wrapped, wrapped, op, n)
	clause, bindings := b.grammar.compileWheres(b.wheres, 0)
	if clause != "" {
		sqlStr += " WHERE " + clause
	}
	result, err := b.executor.Exec(sqlStr, bindings...)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// Delete deletes matching rows and returns the number of affected rows.
func (b *Builder) Delete() (int64, error) {
	sqlStr, bindings := b.grammar.CompileDelete(b)
	result, err := b.executor.Exec(sqlStr, bindings...)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// ScanRows converts *sql.Rows into a slice of maps keyed by column name.
// []byte values are converted to string for ergonomic use.
func ScanRows(rows *sql.Rows) ([]map[string]any, error) {
	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	var results []map[string]any
	for rows.Next() {
		values := make([]any, len(columns))
		pointers := make([]any, len(columns))
		for i := range values {
			pointers[i] = &values[i]
		}
		if err := rows.Scan(pointers...); err != nil {
			return nil, err
		}
		row := make(map[string]any, len(columns))
		for i, c := range columns {
			v := values[i]
			if bytes, ok := v.([]byte); ok {
				v = string(bytes)
			}
			row[c] = v
		}
		results = append(results, row)
	}
	return results, rows.Err()
}

func flatten(values []any) []any {
	// Allow passing a single slice: WhereIn("id", []any{1,2,3}) or WhereIn("id", 1, 2, 3).
	if len(values) == 1 {
		if inner, ok := values[0].([]any); ok {
			return inner
		}
	}
	return values
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	// Deterministic order keeps compiled SQL stable for multi-row inserts and tests.
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j] < keys[j-1]; j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}
	return keys
}

func toInt64(v any) int64 {
	switch n := v.(type) {
	case int64:
		return n
	case int:
		return int64(n)
	case int32:
		return int64(n)
	case float64:
		return int64(n)
	case []byte:
		var out int64
		fmt.Sscanf(string(n), "%d", &out)
		return out
	case string:
		var out int64
		fmt.Sscanf(n, "%d", &out)
		return out
	}
	return 0
}

func toFloat64(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int64:
		return float64(n)
	case int:
		return float64(n)
	case []byte:
		var out float64
		fmt.Sscanf(string(n), "%f", &out)
		return out
	case string:
		var out float64
		fmt.Sscanf(n, "%f", &out)
		return out
	}
	return 0
}

// neutralizePlaceholders rewrites $N placeholders back to ? so a compiled
// subquery can be embedded in a raw where clause, whose compiler renumbers
// placeholders for the active driver.
func neutralizePlaceholders(sqlStr string) string {
	var sb strings.Builder
	i := 0
	for i < len(sqlStr) {
		if sqlStr[i] == '$' && i+1 < len(sqlStr) && sqlStr[i+1] >= '0' && sqlStr[i+1] <= '9' {
			sb.WriteByte('?')
			i++
			for i < len(sqlStr) && sqlStr[i] >= '0' && sqlStr[i] <= '9' {
				i++
			}
			continue
		}
		sb.WriteByte(sqlStr[i])
		i++
	}
	return sb.String()
}

func (b *Builder) addExists(sub *Builder, boolean string, not bool) *Builder {
	subSQL, bindings := sub.ToSQL()
	op := "EXISTS"
	if not {
		op = "NOT EXISTS"
	}
	b.wheres = append(b.wheres, where{
		kind:    whereRaw,
		boolean: boolean,
		rawSQL:  op + " (" + neutralizePlaceholders(subSQL) + ")",
		values:  bindings,
	})
	return b
}

// WhereExists adds a WHERE EXISTS (subquery) clause:
//
//	sub := query.New(driver, nil).Table("posts").
//	    WhereColumn("posts.user_id", "=", "users.id")
//	users := query.New(driver, db).Table("users").WhereExists(sub)
func (b *Builder) WhereExists(sub *Builder) *Builder {
	return b.addExists(sub, "AND", false)
}

// WhereNotExists adds a WHERE NOT EXISTS (subquery) clause.
func (b *Builder) WhereNotExists(sub *Builder) *Builder {
	return b.addExists(sub, "AND", true)
}

// OrWhereExists adds an OR EXISTS (subquery) clause.
func (b *Builder) OrWhereExists(sub *Builder) *Builder {
	return b.addExists(sub, "OR", false)
}

// WhereInSub adds WHERE column IN (subquery).
func (b *Builder) WhereInSub(column string, sub *Builder) *Builder {
	subSQL, bindings := sub.ToSQL()
	b.wheres = append(b.wheres, where{
		kind:    whereRaw,
		boolean: "AND",
		rawSQL:  b.grammar.Wrap(column) + " IN (" + neutralizePlaceholders(subSQL) + ")",
		values:  bindings,
	})
	return b
}
