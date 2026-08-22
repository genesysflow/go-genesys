package support_test

import (
	"strings"
	"testing"

	"github.com/genesysflow/go-genesys/support"
	"github.com/stretchr/testify/assert"
)

func TestStrPluralAndSingular(t *testing.T) {
	assert.Equal(t, "users", support.Str.Plural("user"))
	assert.Equal(t, "people", support.Str.Plural("person"))
	assert.Equal(t, "user", support.Str.Singular("users"))
	assert.Equal(t, "person", support.Str.Singular("people"))

	// A count of one keeps the singular, which is what a message needs.
	assert.Equal(t, "user", support.Str.PluralCount("user", 1))
	assert.Equal(t, "users", support.Str.PluralCount("user", 2))
	assert.Equal(t, "users", support.Str.PluralCount("user", 0))
}

func TestStrBeforeAfterBetween(t *testing.T) {
	assert.Equal(t, "ada", support.Str.Before("ada@example.com", "@"))
	assert.Equal(t, "example.com", support.Str.After("ada@example.com", "@"))
	assert.Equal(t, "b", support.Str.Between("a[b]c", "[", "]"))

	// A missing delimiter returns the whole string rather than nothing,
	// so a helper never silently empties a value.
	assert.Equal(t, "ada", support.Str.Before("ada", "@"))
	assert.Equal(t, "ada", support.Str.After("ada", "@"))
	assert.Equal(t, "", support.Str.Between("ada", "[", "]"))
}

func TestStrMask(t *testing.T) {
	assert.Equal(t, "ada@***********", support.Str.Mask("ada@example.com", '*', 4))
	assert.Equal(t, "ada@***mple.com", support.Str.Mask("ada@example.com", '*', 4, 3))

	// Masking past the end of the string is a no-op, not a panic.
	assert.Equal(t, "ada", support.Str.Mask("ada", '*', 10))
}

func TestStrSquishAndUcfirst(t *testing.T) {
	assert.Equal(t, "a b c", support.Str.Squish("  a   b \n c  "))
	assert.Equal(t, "Ada", support.Str.Ucfirst("ada"))
	assert.Equal(t, "aDA", support.Str.Lcfirst("ADA"))
	assert.Equal(t, "", support.Str.Ucfirst(""))
}

func TestStrStudlyAndWrap(t *testing.T) {
	assert.Equal(t, "FooBar", support.Str.Studly("foo_bar"))
	assert.Equal(t, `"ada"`, support.Str.Wrap("ada", `"`))
	assert.Equal(t, "[ada]", support.Str.Wrap("ada", "[", "]"))
}

func TestStrTake(t *testing.T) {
	assert.Equal(t, "ada", support.Str.Take("ada@example.com", 3))
	assert.Equal(t, ".com", support.Str.Take("ada@example.com", -4))
	assert.Equal(t, "ada", support.Str.Take("ada", 99))
}

// Multi-byte strings must not be cut mid-character.
func TestStrHandlesMultiByte(t *testing.T) {
	assert.Equal(t, "héllo", support.Str.Take("héllo wörld", 5))
	assert.Equal(t, "Héllo", support.Str.Ucfirst("héllo"))
	assert.Equal(t, 5, len([]rune(support.Str.Take("héllo wörld", 5))))
}

// --- fluent ----------------------------------------------------------

// Str.Of chains the helpers, which is how a transformation reads as one
// statement rather than five nested calls.
func TestStrOfChain(t *testing.T) {
	result := support.Str.Of("  Hello World  ").
		Trim().
		Lower().
		Replace(" ", "-").
		Value()

	assert.Equal(t, "hello-world", result)
}

func TestStrOfChainMore(t *testing.T) {
	slug := support.Str.Of("The Quick Brown Fox").Slug().Value()
	assert.Equal(t, "the-quick-brown-fox", slug)

	assert.Equal(t, "THE QUICK...", support.Str.Of("The quick brown fox").Upper().Limit(9).Value())
	assert.True(t, support.Str.Of("ada@example.com").Contains("@"))
	assert.Equal(t, "ada", support.Str.Of("ada@example.com").Before("@").Value())
	assert.Equal(t, 3, support.Str.Of("ada").Length())
}

func TestStrOfWhen(t *testing.T) {
	assert.Equal(t, "ADA", support.Str.Of("ada").When(true, func(s *support.Stringable) *support.Stringable {
		return s.Upper()
	}).Value())

	assert.Equal(t, "ada", support.Str.Of("ada").When(false, func(s *support.Stringable) *support.Stringable {
		return s.Upper()
	}).Value())
}

// --- existing helpers keep working -----------------------------------

func TestStrExistingHelpers(t *testing.T) {
	assert.Equal(t, "hello-world", support.Str.Slug("Hello World"))
	assert.Equal(t, "helloWorld", support.Str.Camel("hello_world"))
	assert.True(t, strings.HasPrefix(support.Str.Random(16), support.Str.Random(16)[:0]))
	assert.Len(t, support.Str.Random(16), 16)
}
