package lang

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Placeholder keys apply longest-first: ":name" must not corrupt
// ":name_full".
func TestPlaceholderPrefixesDoNotCollide(t *testing.T) {
	out := replacePlaceholders("Hello :name_full (:name)", map[string]string{
		"name":      "X",
		"name_full": "Xavier Yellow",
	})
	assert.Equal(t, "Hello Xavier Yellow (X)", out)
}
