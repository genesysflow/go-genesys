// Package query provides the fluent query builder.
package query

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/genesysflow/go-genesys/contracts"
)

// Builder is a fluent SQL query builder.
type Builder struct {
	db       *sql.DB
	tx       *sql.Tx
	grammar  contracts.Grammar
	table    string
	columns  []string
	distinct bool
	joins    []joinClause
	wheres   []whereClause
	groups   []string
	havings  []havingClause
	orders   []orderClause
	limit    *int
	offset   *int
	bindings []any
	err      error
	ctx      context.Context
}

type joinClause struct {
	joinType string
	table    string
	first    string
	operator string
	second   string
}

type whereClause struct {
	column     string
	operator   string
	value      any
	boolean    string // "AND" or "OR"
	whereType  string // "basic", "in", "null", "between", "raw"
	values     []any  // for IN clauses
	low, high  any    // for BETWEEN
	rawSQL     string // for raw where
	rawBinding []any
}

type havingClause struct {
	column     string
	operator   string
	value      any
	rawSQL     string
	rawBinding []any
}

type orderClause struct {
	column    string
	direction string
	rawSQL    string
}

// NewBuilder creates a new query builder.
func NewBuilder(db *sql.DB, grammar contracts.Grammar, table string) *Builder {
	return &Builder{
		db:       db,
		grammar:  grammar,
		table:    table,
		columns:  []string{"*"},
		bindings: make([]any, 0),
	}
}

// NewBuilderWithTx creates a new query builder with a transaction.
func NewBuilderWithTx(tx *sql.Tx, grammar contracts.Grammar, table string) *Builder {
	return &Builder{
		tx:       tx,
		grammar:  grammar,
		table:    table,
		columns:  []string{"*"},
		bindings: make([]any, 0),
	}
}

// WithContext sets the context for the query.
func (b *Builder) WithContext(ctx context.Context) contracts.QueryBuilder {
	b.ctx = ctx
	return b
}

// Select specifies the columns to select.
func (b *Builder) Select(columns ...string) contracts.QueryBuilder {
	if len(columns) > 0 {
		b.columns = columns
	}
	return b
}

// SelectRaw adds a raw select expression.
func (b *Builder) SelectRaw(expression string, bindings ...any) contracts.QueryBuilder {
	b.columns = append(b.columns, expression)
	b.bindings = append(b.bindings, bindings...)
	return b
}

// Distinct constrains the query to return distinct results.
func (b *Builder) Distinct() contracts.QueryBuilder {
	b.distinct = true
	return b
}

// From sets the table to query from.
func (b *Builder) From(table string) contracts.QueryBuilder {
	b.table = table
	return b
}

// Join adds a join clause.
func (b *Builder) Join(table, first, operator, second string) contracts.QueryBuilder {
	b.joins = append(b.joins, joinClause{
		joinType: "INNER",
		table:    table,
		first:    first,
		operator: operator,
		second:   second,
	})
	return b
}

// LeftJoin adds a left join clause.
func (b *Builder) LeftJoin(table, first, operator, second string) contracts.QueryBuilder {
	b.joins = append(b.joins, joinClause{
		joinType: "LEFT",
		table:    table,
		first:    first,
		operator: operator,
		second:   second,
	})
	return b
}

// RightJoin adds a right join clause.
func (b *Builder) RightJoin(table, first, operator, second string) contracts.QueryBuilder {
	b.joins = append(b.joins, joinClause{
		joinType: "RIGHT",
		table:    table,
		first:    first,
		operator: operator,
		second:   second,
	})
	return b
}

// CrossJoin adds a cross join clause.
func (b *Builder) CrossJoin(table string) contracts.QueryBuilder {
	b.joins = append(b.joins, joinClause{
		joinType: "CROSS",
		table:    table,
	})
	return b
}

// Where adds a where clause.
func (b *Builder) Where(column string, operator string, value any) contracts.QueryBuilder {
	b.wheres = append(b.wheres, whereClause{
		column:    column,
		operator:  operator,
		value:     value,
		boolean:   "AND",
		whereType: "basic",
	})
	return b
}

// OrWhere adds an or where clause.
func (b *Builder) OrWhere(column string, operator string, value any) contracts.QueryBuilder {
	b.wheres = append(b.wheres, whereClause{
		column:    column,
		operator:  operator,
		value:     value,
		boolean:   "OR",
		whereType: "basic",
	})
	return b
}

// WhereIn adds a where in clause.
func (b *Builder) WhereIn(column string, values []any) contracts.QueryBuilder {
	b.wheres = append(b.wheres, whereClause{
		column:    column,
		values:    values,
		boolean:   "AND",
		whereType: "in",
	})
	return b
}

// WhereNotIn adds a where not in clause.
func (b *Builder) WhereNotIn(column string, values []any) contracts.QueryBuilder {
	b.wheres = append(b.wheres, whereClause{
		column:    column,
		values:    values,
		boolean:   "AND",
		whereType: "not_in",
	})
	return b
}

// WhereNull adds a where null clause.
func (b *Builder) WhereNull(column string) contracts.QueryBuilder {
	b.wheres = append(b.wheres, whereClause{
		column:    column,
		boolean:   "AND",
		whereType: "null",
	})
	return b
}

// WhereNotNull adds a where not null clause.
func (b *Builder) WhereNotNull(column string) contracts.QueryBuilder {
	b.wheres = append(b.wheres, whereClause{
		column:    column,
		boolean:   "AND",
		whereType: "not_null",
	})
	return b
}

// WhereBetween adds a where between clause.
func (b *Builder) WhereBetween(column string, low, high any) contracts.QueryBuilder {
	b.wheres = append(b.wheres, whereClause{
		column:    column,
		low:       low,
		high:      high,
		boolean:   "AND",
		whereType: "between",
	})
	return b
}

// WhereRaw adds a raw where clause.
func (b *Builder) WhereRaw(sqlStr string, bindings ...any) contracts.QueryBuilder {
	b.wheres = append(b.wheres, whereClause{
		rawSQL:     sqlStr,
		rawBinding: bindings,
		boolean:    "AND",
		whereType:  "raw",
	})
	return b
}

// GroupBy adds a group by clause.
func (b *Builder) GroupBy(columns ...string) contracts.QueryBuilder {
	b.groups = append(b.groups, columns...)
	return b
}

// Having adds a having clause.
func (b *Builder) Having(column, operator string, value any) contracts.QueryBuilder {
	b.havings = append(b.havings, havingClause{
		column:   column,
		operator: operator,
		value:    value,
	})
	return b
}

// HavingRaw adds a raw having clause.
func (b *Builder) HavingRaw(sqlStr string, bindings ...any) contracts.QueryBuilder {
	b.havings = append(b.havings, havingClause{
		rawSQL:     sqlStr,
		rawBinding: bindings,
	})
	return b
}

// OrderBy adds an order by clause.
func (b *Builder) OrderBy(column, direction string) contracts.QueryBuilder {
	dir := strings.ToUpper(direction)
	if dir != "ASC" && dir != "DESC" {
		dir = "ASC"
	}
	b.orders = append(b.orders, orderClause{
		column:    column,
		direction: dir,
	})
	return b
}

// OrderByDesc adds descending order by clause.
func (b *Builder) OrderByDesc(column string) contracts.QueryBuilder {
	return b.OrderBy(column, "DESC")
}

// OrderByRaw adds a raw order by clause.
func (b *Builder) OrderByRaw(sqlStr string, bindings ...any) contracts.QueryBuilder {
	b.orders = append(b.orders, orderClause{
		rawSQL: sqlStr,
	})
	b.bindings = append(b.bindings, bindings...)
	return b
}

// Limit limits the number of results.
func (b *Builder) Limit(limit int) contracts.QueryBuilder {
	b.limit = &limit
	return b
}

// Offset sets the offset for the query.
func (b *Builder) Offset(offset int) contracts.QueryBuilder {
	b.offset = &offset
	return b
}

// Take is an alias for Limit.
func (b *Builder) Take(count int) contracts.QueryBuilder {
	return b.Limit(count)
}

// Skip is an alias for Offset.
func (b *Builder) Skip(count int) contracts.QueryBuilder {
	return b.Offset(count)
}

// ForPage paginates the results.
func (b *Builder) ForPage(page, perPage int) contracts.QueryBuilder {
	if page < 1 {
		page = 1
	}
	return b.Offset((page - 1) * perPage).Limit(perPage)
}

// Get executes the query and returns all results.
func (b *Builder) Get(dest ...any) ([]map[string]any, error) {
	sqlStr, bindings := b.ToSQL()

	rows, err := b.query(sqlStr, bindings...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanRows(rows)
}

// First returns the first result.
func (b *Builder) First(dest ...any) (map[string]any, error) {
	b.Limit(1)
	results, err := b.Get(dest...)
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, nil
	}
	return results[0], nil
}

// Find finds a record by primary key.
func (b *Builder) Find(id any, dest ...any) (map[string]any, error) {
	return b.Where("id", "=", id).First(dest...)
}

// Value returns a single column value.
func (b *Builder) Value(column string) (any, error) {
	b.columns = []string{column}
	result, err := b.First()
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
	}
	return result[column], nil
}

// Pluck returns a slice of values for a column.
func (b *Builder) Pluck(column string) ([]any, error) {
	b.columns = []string{column}
	results, err := b.Get()
	if err != nil {
		return nil, err
	}

	values := make([]any, 0, len(results))
	for _, row := range results {
		if v, ok := row[column]; ok {
			values = append(values, v)
		}
	}
	return values, nil
}

// Exists checks if any records exist.
func (b *Builder) Exists() (bool, error) {
	count, err := b.Count()
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// DoesntExist checks if no records exist.
func (b *Builder) DoesntExist() (bool, error) {
	exists, err := b.Exists()
	return !exists, err
}

// Count returns the count of records.
func (b *Builder) Count() (int64, error) {
	return b.aggregate("COUNT", "*")
}

// Max returns the max value of a column.
func (b *Builder) Max(column string) (any, error) {
	return b.aggregateValue("MAX", column)
}

// Min returns the min value of a column.
func (b *Builder) Min(column string) (any, error) {
	return b.aggregateValue("MIN", column)
}

// Sum returns the sum of a column.
func (b *Builder) Sum(column string) (float64, error) {
	val, err := b.aggregateValue("SUM", column)
	if err != nil {
		return 0, err
	}
	if val == nil {
		return 0, nil
	}
	switch v := val.(type) {
	case float64:
		return v, nil
	case int64:
		return float64(v), nil
	case int:
		return float64(v), nil
	default:
		return 0, fmt.Errorf("unexpected type for sum: %T", val)
	}
}

// Avg returns the average of a column.
func (b *Builder) Avg(column string) (float64, error) {
	val, err := b.aggregateValue("AVG", column)
	if err != nil {
		return 0, err
	}
	if val == nil {
		return 0, nil
	}
	switch v := val.(type) {
	case float64:
		return v, nil
	case int64:
		return float64(v), nil
	default:
		return 0, fmt.Errorf("unexpected type for avg: %T", val)
	}
}

func (b *Builder) aggregate(fn, column string) (int64, error) {
	original := b.columns
	b.columns = []string{fmt.Sprintf("%s(%s) AS aggregate", fn, column)}

	result, err := b.First()
	b.columns = original

	if err != nil {
		return 0, err
	}
	if result == nil {
		return 0, nil
	}

	if v, ok := result["aggregate"]; ok {
		switch val := v.(type) {
		case int64:
			return val, nil
		case int:
			return int64(val), nil
		default:
			return 0, nil
		}
	}
	return 0, nil
}

func (b *Builder) aggregateValue(fn, column string) (any, error) {
	original := b.columns
	b.columns = []string{fmt.Sprintf("%s(%s) AS aggregate", fn, b.grammar.WrapColumn(column))}

	result, err := b.First()
	b.columns = original

	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
	}

	return result["aggregate"], nil
}

// Insert inserts a record.
func (b *Builder) Insert(values map[string]any) (int64, error) {
	sqlStr, bindings := b.grammar.CompileInsert(b, values)
	result, err := b.exec(sqlStr, bindings...)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// InsertGetId inserts a record and returns the inserted ID.
func (b *Builder) InsertGetId(values map[string]any) (int64, error) {
	sqlStr, bindings := b.grammar.CompileInsert(b, values)

	// For PostgreSQL, we need RETURNING id
	if _, ok := b.grammar.(*PostgresGrammar); ok {
		sqlStr = sqlStr + " RETURNING id"
		var id int64
		var err error
		if b.tx != nil {
			err = b.tx.QueryRow(sqlStr, bindings...).Scan(&id)
		} else {
			err = b.db.QueryRow(sqlStr, bindings...).Scan(&id)
		}
		if err != nil {
			return 0, err
		}
		return id, nil
	}

	// For SQLite and other databases, use LastInsertId
	result, err := b.exec(sqlStr, bindings...)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

// InsertBatch inserts multiple records.
func (b *Builder) InsertBatch(records []map[string]any) (int64, error) {
	if len(records) == 0 {
		return 0, nil
	}

	var totalAffected int64
	for _, record := range records {
		affected, err := b.Clone().(*Builder).Insert(record)
		if err != nil {
			return totalAffected, err
		}
		totalAffected += affected
	}
	return totalAffected, nil
}

// Update updates records.
func (b *Builder) Update(values map[string]any) (int64, error) {
	sqlStr, bindings := b.grammar.CompileUpdate(b, values)
	result, err := b.exec(sqlStr, bindings...)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// Increment increments a column.
func (b *Builder) Increment(column string, amount ...int) (int64, error) {
	inc := 1
	if len(amount) > 0 {
		inc = amount[0]
	}
	return b.Update(map[string]any{
		column: RawExpression(fmt.Sprintf("%s + %d", b.grammar.WrapColumn(column), inc)),
	})
}

// Decrement decrements a column.
func (b *Builder) Decrement(column string, amount ...int) (int64, error) {
	dec := 1
	if len(amount) > 0 {
		dec = amount[0]
	}
	return b.Update(map[string]any{
		column: RawExpression(fmt.Sprintf("%s - %d", b.grammar.WrapColumn(column), dec)),
	})
}

// Delete deletes records.
func (b *Builder) Delete() (int64, error) {
	sqlStr, bindings := b.grammar.CompileDelete(b)
	result, err := b.exec(sqlStr, bindings...)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// Truncate truncates the table.
func (b *Builder) Truncate() error {
	sqlStr := fmt.Sprintf("TRUNCATE TABLE %s", b.grammar.WrapTable(b.table))
	_, err := b.exec(sqlStr)
	return err
}

// ToSQL returns the generated SQL.
func (b *Builder) ToSQL() (string, []any) {
	return b.grammar.CompileSelect(b)
}

// Clone clones the query builder.
func (b *Builder) Clone() contracts.QueryBuilder {
	clone := &Builder{
		db:       b.db,
		tx:       b.tx,
		grammar:  b.grammar,
		table:    b.table,
		columns:  make([]string, len(b.columns)),
		distinct: b.distinct,
		joins:    make([]joinClause, len(b.joins)),
		wheres:   make([]whereClause, len(b.wheres)),
		groups:   make([]string, len(b.groups)),
		havings:  make([]havingClause, len(b.havings)),
		orders:   make([]orderClause, len(b.orders)),
		bindings: make([]any, len(b.bindings)),
	}
	copy(clone.columns, b.columns)
	copy(clone.joins, b.joins)
	copy(clone.wheres, b.wheres)
	copy(clone.groups, b.groups)
	copy(clone.havings, b.havings)
	copy(clone.orders, b.orders)
	copy(clone.bindings, b.bindings)
	clone.err = b.err
	clone.ctx = b.ctx
	if b.limit != nil {
		limit := *b.limit
		clone.limit = &limit
	}
	if b.offset != nil {
		offset := *b.offset
		clone.offset = &offset
	}
	return clone
}

// SetError sets an error on the builder.
func (b *Builder) SetError(err error) {
	b.err = err
}

// query executes a query.
func (b *Builder) query(sqlStr string, bindings ...any) (*sql.Rows, error) {
	if b.err != nil {
		return nil, b.err
	}
	ctx := b.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	if b.tx != nil {
		return b.tx.QueryContext(ctx, sqlStr, bindings...)
	}
	return b.db.QueryContext(ctx, sqlStr, bindings...)
}

// exec executes a statement.
func (b *Builder) exec(sqlStr string, bindings ...any) (sql.Result, error) {
	if b.err != nil {
		return nil, b.err
	}
	ctx := b.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	if b.tx != nil {
		return b.tx.ExecContext(ctx, sqlStr, bindings...)
	}
	return b.db.ExecContext(ctx, sqlStr, bindings...)
}

// GetTable returns the table name.
func (b *Builder) GetTable() string {
	return b.table
}

// GetColumns returns the selected columns.
func (b *Builder) GetColumns() []string {
	return b.columns
}

// IsDistinct returns whether the query is distinct.
func (b *Builder) IsDistinct() bool {
	return b.distinct
}

// GetJoins returns the join clauses.
func (b *Builder) GetJoins() []joinClause {
	return b.joins
}

// GetWheres returns the where clauses.
func (b *Builder) GetWheres() []whereClause {
	return b.wheres
}

// GetGroups returns the group by columns.
func (b *Builder) GetGroups() []string {
	return b.groups
}

// GetHavings returns the having clauses.
func (b *Builder) GetHavings() []havingClause {
	return b.havings
}

// GetOrders returns the order clauses.
func (b *Builder) GetOrders() []orderClause {
	return b.orders
}

// GetLimit returns the limit.
func (b *Builder) GetLimit() *int {
	return b.limit
}

// GetOffset returns the offset.
func (b *Builder) GetOffset() *int {
	return b.offset
}

// scanRows converts sql.Rows to a slice of maps.
func scanRows(rows *sql.Rows) ([]map[string]any, error) {
	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	results := make([]map[string]any, 0)

	for rows.Next() {
		values := make([]any, len(columns))
		valuePtrs := make([]any, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			return nil, err
		}

		row := make(map[string]any)
		for i, col := range columns {
			val := values[i]
			// Convert []byte to string for easier handling
			if b, ok := val.([]byte); ok {
				row[col] = string(b)
			} else {
				row[col] = val
			}
		}
		results = append(results, row)
	}

	return results, rows.Err()
}

// RawExpression represents a raw SQL expression.
type RawExpression string
