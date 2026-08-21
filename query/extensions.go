package query

import (
	"fmt"
	"regexp"
	"strings"
)

// --- pessimistic locking ---

// LockForUpdate acquires an exclusive row lock (SELECT ... FOR UPDATE)
// so concurrent transactions cannot modify or lock the selected rows
// until this transaction ends. SQLite locks at database level already,
// so the clause is omitted there.
func (b *Builder) LockForUpdate() *Builder {
	b.lock = "update"
	return b
}

// SharedLock acquires a shared row lock: other transactions may read
// but not modify the selected rows until this transaction ends.
func (b *Builder) SharedLock() *Builder {
	b.lock = "shared"
	return b
}

// compileLock renders the driver's lock clause.
func (g *Grammar) compileLock(lock string) string {
	if lock == "" || g.driver == "sqlite" || g.driver == "sqlite3" {
		return ""
	}
	switch lock {
	case "update":
		return "FOR UPDATE"
	case "shared":
		if g.isPostgres() {
			return "FOR SHARE"
		}
		return "LOCK IN SHARE MODE" // MySQL / MariaDB
	}
	return ""
}

// --- unions ---

type unionPart struct {
	builder *Builder
	all     bool
}

// Union appends another query with UNION (duplicates removed). ORDER
// BY / LIMIT / OFFSET on the receiving builder apply to the combined
// result:
//
//	recent := query.New(d, db).Table("posts").Select("title")
//	all := query.New(d, db).Table("pages").Select("title").Union(recent).OrderBy("title")
func (b *Builder) Union(other *Builder) *Builder {
	b.unions = append(b.unions, unionPart{builder: other})
	return b
}

// UnionAll appends another query with UNION ALL (duplicates kept).
func (b *Builder) UnionAll(other *Builder) *Builder {
	b.unions = append(b.unions, unionPart{builder: other, all: true})
	return b
}

// --- joins ---

// CrossJoin adds a CROSS JOIN (cartesian product) with the given table.
func (b *Builder) CrossJoin(table string) *Builder {
	b.joins = append(b.joins, join{kind: "CROSS", table: table})
	return b
}

// --- JSON path queries ---

var jsonPathPattern = regexp.MustCompile(`^[A-Za-z0-9_]+(\.[A-Za-z0-9_]+)*$`)

// WhereJSON constrains a JSON column by a dot-notation path - Laravel's
// where('meta->color', ...). The extracted value compares as text on
// PostgreSQL and as a JSON scalar on MySQL/SQLite:
//
//	q.WhereJSON("meta", "color", "red")
//	q.WhereJSON("meta", "specs.weight", ">", 10)
func (b *Builder) WhereJSON(column, path string, args ...any) *Builder {
	operator := "="
	var value any
	switch len(args) {
	case 1:
		value = args[0]
	case 2:
		operator = assertOperator(fmt.Sprint(args[0]))
		value = args[1]
	default:
		panic("query: WhereJSON expects (column, path, value) or (column, path, operator, value)")
	}

	expr := b.grammar.jsonExtract(column, path)
	b.wheres = append(b.wheres, where{
		kind:    whereRaw,
		boolean: "AND",
		rawSQL:  expr + " " + operator + " ?",
		values:  []any{value},
	})
	return b
}

// jsonExtract renders the driver's JSON path expression. The path is
// interpolated, so it is validated against a strict identifier pattern.
func (g *Grammar) jsonExtract(column, path string) string {
	if !jsonPathPattern.MatchString(path) {
		panic(fmt.Sprintf("query: illegal JSON path %q (dot-separated identifiers only)", path))
	}
	if g.isPostgres() {
		segments := strings.Split(path, ".")
		return g.Wrap(column) + " #>> '{" + strings.Join(segments, ",") + "}'"
	}
	return "json_extract(" + g.Wrap(column) + ", '$." + path + "')"
}

// --- bulk upsert ---

// Upsert inserts rows, updating existing ones that collide on the
// uniqueBy columns - Laravel's upsert(). With update nil, every
// non-unique column is updated; pass explicit columns to narrow it.
// The uniqueBy columns must be covered by a unique index.
//
//	q.Table("flights").Upsert(rows, []string{"code"}, []string{"price"})
func (b *Builder) Upsert(rows []map[string]any, uniqueBy []string, update []string) (int64, error) {
	if len(rows) == 0 {
		return 0, nil
	}
	if len(uniqueBy) == 0 {
		return 0, fmt.Errorf("query: Upsert needs at least one uniqueBy column")
	}
	columns := sortedKeys(rows[0])
	if update == nil {
		unique := make(map[string]bool, len(uniqueBy))
		for _, c := range uniqueBy {
			unique[c] = true
		}
		for _, c := range columns {
			if !unique[c] {
				update = append(update, c)
			}
		}
	}

	sqlStr := b.grammar.CompileUpsert(b.table, columns, len(rows), uniqueBy, update)
	var bindings []any
	for _, row := range rows {
		for _, c := range columns {
			bindings = append(bindings, row[c])
		}
	}
	result, err := b.executor.Exec(sqlStr, bindings...)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// CompileUpsert compiles the driver's INSERT ... ON CONFLICT/DUPLICATE
// upsert form.
func (g *Grammar) CompileUpsert(table string, columns []string, rows int, uniqueBy, update []string) string {
	insert := g.CompileInsert(table, columns, rows)

	if g.driver == "mysql" {
		if len(update) == 0 {
			// Nothing to update: touch the first unique column with its
			// own value so the statement stays a no-op on conflicts.
			update = uniqueBy[:1]
			sets := g.Wrap(update[0]) + " = " + g.Wrap(update[0])
			return insert + " ON DUPLICATE KEY UPDATE " + sets
		}
		sets := make([]string, len(update))
		for i, c := range update {
			sets[i] = g.Wrap(c) + " = VALUES(" + g.Wrap(c) + ")"
		}
		return insert + " ON DUPLICATE KEY UPDATE " + strings.Join(sets, ", ")
	}

	// PostgreSQL and SQLite share ON CONFLICT ... DO UPDATE.
	conflict := make([]string, len(uniqueBy))
	for i, c := range uniqueBy {
		conflict[i] = g.Wrap(c)
	}
	if len(update) == 0 {
		return insert + " ON CONFLICT (" + strings.Join(conflict, ", ") + ") DO NOTHING"
	}
	sets := make([]string, len(update))
	for i, c := range update {
		sets[i] = g.Wrap(c) + " = excluded." + g.Wrap(c)
	}
	return insert + " ON CONFLICT (" + strings.Join(conflict, ", ") + ") DO UPDATE SET " + strings.Join(sets, ", ")
}
