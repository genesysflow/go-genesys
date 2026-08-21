package validation

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateMapWildcards(t *testing.T) {
	v := New()

	result := v.ValidateMap(map[string]any{
		"name": "Order",
		"items": []any{
			map[string]any{"sku": "ABC1", "count": 2},
			map[string]any{"sku": "!!", "count": 0},
			map[string]any{"count": 1},
		},
	}, map[string]string{
		"name":          "required",
		"items.*.sku":   "required,alphanum",
		"items.*.count": "gte=1",
	})

	require.False(t, result.Passes())
	errs := result.Errors()
	assert.True(t, errs.Has("items.1.sku"), "invalid sku reported under its index")
	assert.True(t, errs.Has("items.1.count"), "count 0 fails gte=1")
	assert.True(t, errs.Has("items.2.sku"), "missing required sku reported")
	assert.False(t, errs.Has("items.0.sku"), "valid elements stay clean")
}

func TestValidateMapWildcardsAllValid(t *testing.T) {
	v := New()
	result := v.ValidateMap(map[string]any{
		"items": []any{
			map[string]any{"sku": "A1"},
			map[string]any{"sku": "B2"},
		},
	}, map[string]string{"items.*.sku": "required,alphanum"})
	assert.True(t, result.Passes())
}
