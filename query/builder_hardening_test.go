package query_test

import (
	"testing"

	"github.com/genesysflow/go-genesys/query"
	"github.com/stretchr/testify/assert"
)

// The operator position is interpolated into SQL, so anything outside
// the whitelist must be rejected loudly rather than compiled.
func TestIllegalOperatorPanics(t *testing.T) {
	assert.PanicsWithValue(t,
		`query: illegal operator "= 18 OR 1=1 --"`,
		func() {
			query.New("sqlite", nil).Table("users").Where("age", "= 18 OR 1=1 --", 18)
		})

	assert.NotPanics(t, func() {
		query.New("sqlite", nil).Table("users").
			Where("age", ">=", 18).
			Where("name", "LIKE", "a%").
			WhereColumn("a", "!=", "b").
			Having("total", "<", 10)
	}, "whitelisted operators pass, case-insensitively")
}
