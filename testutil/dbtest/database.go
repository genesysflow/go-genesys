// Package dbtest provides database assertions for tests: the answers to
// "did that action write the row it was supposed to?"
package dbtest

import (
	"fmt"
	"sort"
	"strings"

	"github.com/genesysflow/go-genesys/database"
	"github.com/genesysflow/go-genesys/query"
)

// TestingT is the subset of *testing.T the assertions use.
type TestingT interface {
	Helper()
	Errorf(format string, args ...any)
}

// AssertDatabaseHas fails unless a row matching every column is present,
// Laravel's assertDatabaseHas:
//
//	testutil.AssertDatabaseHas(t, "users", map[string]any{"email": "ada@example.com"})
func AssertDatabaseHas(t TestingT, table string, columns map[string]any) {
	t.Helper()

	count, err := countMatching(table, columns)
	if err != nil {
		t.Errorf("database assertion on %s failed: %v", table, err)
		return
	}
	if count == 0 {
		t.Errorf("expected a row in %s matching %s, found none", table, renderColumns(columns))
	}
}

// AssertDatabaseMissing fails when a row matching every column exists.
func AssertDatabaseMissing(t TestingT, table string, columns map[string]any) {
	t.Helper()

	count, err := countMatching(table, columns)
	if err != nil {
		t.Errorf("database assertion on %s failed: %v", table, err)
		return
	}
	if count > 0 {
		t.Errorf("expected no row in %s matching %s, found %d", table, renderColumns(columns), count)
	}
}

// AssertDatabaseCount fails unless the table holds exactly expected rows.
func AssertDatabaseCount(t TestingT, table string, expected int64) {
	t.Helper()

	count, err := countMatching(table, nil)
	if err != nil {
		t.Errorf("database assertion on %s failed: %v", table, err)
		return
	}
	if count != expected {
		t.Errorf("expected %s to hold %d rows, found %d", table, expected, count)
	}
}

// AssertSoftDeleted fails unless the matching row is soft-deleted.
func AssertSoftDeleted(t TestingT, table string, columns map[string]any) {
	t.Helper()

	builder, err := builderFor(table, columns)
	if err != nil {
		t.Errorf("database assertion on %s failed: %v", table, err)
		return
	}

	count, err := builder.WhereNotNull("deleted_at").Count()
	if err != nil {
		t.Errorf("database assertion on %s failed: %v", table, err)
		return
	}
	if count == 0 {
		t.Errorf("expected a soft-deleted row in %s matching %s, found none", table, renderColumns(columns))
	}
}

// AssertNotSoftDeleted fails when the matching row is soft-deleted.
func AssertNotSoftDeleted(t TestingT, table string, columns map[string]any) {
	t.Helper()

	builder, err := builderFor(table, columns)
	if err != nil {
		t.Errorf("database assertion on %s failed: %v", table, err)
		return
	}

	count, err := builder.WhereNull("deleted_at").Count()
	if err != nil {
		t.Errorf("database assertion on %s failed: %v", table, err)
		return
	}
	if count == 0 {
		t.Errorf("expected a live row in %s matching %s, found none", table, renderColumns(columns))
	}
}

// countMatching counts the rows matching every column.
func countMatching(table string, columns map[string]any) (int64, error) {
	builder, err := builderFor(table, columns)
	if err != nil {
		return 0, err
	}
	return builder.Count()
}

// builderFor starts a query against the default connection. A missing
// database is reported rather than read as "no matching row", which
// would turn a setup mistake into a passing assertion.
func builderFor(table string, columns map[string]any) (*query.Builder, error) {
	manager := database.Default()
	if manager == nil {
		return nil, fmt.Errorf("no default database is configured")
	}

	conn := manager.Connection()
	if err := conn.Error(); err != nil {
		return nil, fmt.Errorf("the database connection could not be opened: %w", err)
	}

	builder := query.New(conn.Driver(), conn).Table(table)
	for column, value := range columns {
		if value == nil {
			builder = builder.WhereNull(column)
			continue
		}
		builder = builder.Where(column, value)
	}
	return builder, nil
}

// renderColumns renders the constraint for a failure message, ordered so
// the message is stable.
func renderColumns(columns map[string]any) string {
	if len(columns) == 0 {
		return "{}"
	}

	keys := make([]string, 0, len(columns))
	for key := range columns {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s=%v", key, columns[key]))
	}
	return "{" + strings.Join(parts, ", ") + "}"
}
