package schema

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Identifiers cannot be sent as bind parameters, so they are necessarily
// interpolated into the statement. Any quote character they contain must be
// escaped by doubling, or the identifier can close the quoted region early and
// have its remainder parsed as SQL.
func TestWrapEscapesEmbeddedQuotes(t *testing.T) {
	grammars := map[string]Grammar{
		"sqlite":   &SQLiteGrammar{},
		"postgres": &PostgresGrammar{},
	}

	// A payload that would close the identifier and append a second statement.
	const payload = `users"; DROP TABLE users; --`

	for name, g := range grammars {
		t.Run(name, func(t *testing.T) {
			wrapped := g.WrapTable(payload)

			assert.True(t, strings.HasPrefix(wrapped, `"`))
			assert.True(t, strings.HasSuffix(wrapped, `"`))
			assert.Contains(t, wrapped, `""`, "the embedded quote must be doubled")

			// Strip the outer quotes; no bare quote may remain inside.
			inner := wrapped[1 : len(wrapped)-1]
			assert.NotContains(t, strings.ReplaceAll(inner, `""`, ""), `"`,
				"identifier must not be able to terminate its own quoting")

			// The same guarantee for column identifiers.
			assert.Contains(t, g.WrapColumn(payload), `""`)
		})
	}
}

// TestCompileTableExistsEscapesStringLiteral covers the string-literal path,
// where a single quote is the terminator rather than a double quote.
func TestCompileTableExistsEscapesStringLiteral(t *testing.T) {
	grammars := map[string]Grammar{
		"sqlite":   &SQLiteGrammar{},
		"postgres": &PostgresGrammar{},
	}

	const payload = `users' OR '1'='1`

	for name, g := range grammars {
		t.Run(name, func(t *testing.T) {
			query := g.CompileTableExists(payload)
			assert.Contains(t, query, `''`, "the embedded quote must be doubled")
			assert.NotContains(t, query, `OR '1'='1'`,
				"the payload must not survive as executable syntax")
		})
	}
}

// TestStringDefaultsAreEscaped: a column default is emitted as a string
// literal and is subject to the same escaping requirement.
//
// The payload's own text still appears in the output — that is the point of a
// literal. What matters is that every quote within it is doubled, so the
// literal stays closed and the payload is data rather than syntax.
func TestStringDefaultsAreEscaped(t *testing.T) {
	for name, g := range map[string]Grammar{
		"sqlite":   &SQLiteGrammar{},
		"postgres": &PostgresGrammar{},
	} {
		t.Run(name, func(t *testing.T) {
			bp := NewBlueprint("items")
			bp.String("label", 50).Default(`it's'; DROP TABLE items; --`)

			sql := g.CompileCreate(bp)

			assert.Contains(t, sql, `DEFAULT 'it''s''; DROP TABLE items; --'`,
				"every quote in the default must be doubled")

			// An unbalanced quote count means some literal is left open, which
			// is exactly how injected syntax escapes into the statement.
			assert.Zero(t, strings.Count(sql, "'")%2,
				"quotes must be balanced, got %q", sql)
		})
	}
}

// TestNormalIdentifiersUnchanged confirms escaping does not disturb ordinary
// names.
func TestNormalIdentifiersUnchanged(t *testing.T) {
	for _, g := range []Grammar{&SQLiteGrammar{}, &PostgresGrammar{}} {
		assert.Equal(t, `"users"`, g.WrapTable("users"))
		assert.Equal(t, `"created_at"`, g.WrapColumn("created_at"))
	}
}
