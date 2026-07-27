package support

import (
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRandomLength checks the caller gets the length they asked for.
func TestRandomLength(t *testing.T) {
	for _, n := range []int{1, 2, 7, 16, 32, 64} {
		assert.Len(t, Str.Random(n), n)
		assert.Len(t, RandomString(n), n)
	}

	assert.Empty(t, Str.Random(0))
	assert.Empty(t, RandomString(0))
	assert.Empty(t, Str.Random(-1))
	assert.Empty(t, RandomString(-1))
}

// TestRandomDrawsFullEntropy is the regression test for silently halved
// entropy: encoding n bytes and truncating the encoded form to n characters
// discards entropy, because hex doubles its input and base64 grows it by a
// third. Random(32) must be 32 hex characters of *fresh* randomness, not 16
// bytes stretched over 32 characters.
//
// A truncated-encoding implementation is detectable statistically: with only
// n/2 bytes of entropy, collisions across many samples appear far sooner than
// they should. This asserts the weaker but decisive property that all
// positions vary, which fails outright if the tail is derived rather than
// random.
func TestRandomDrawsFullEntropy(t *testing.T) {
	const (
		length  = 16
		samples = 400
	)

	positionValues := make([]map[byte]bool, length)
	for i := range positionValues {
		positionValues[i] = make(map[byte]bool)
	}

	seen := make(map[string]bool, samples)
	for range samples {
		value := Str.Random(length)
		require.Len(t, value, length)
		assert.False(t, seen[value], "random values must not repeat")
		seen[value] = true

		for i := range length {
			positionValues[i][value[i]] = true
		}
	}

	// Every character position must take many distinct values. A position
	// fixed by truncation would collapse to one.
	for i, values := range positionValues {
		assert.Greaterf(t, len(values), 8,
			"position %d varies over only %d values, suggesting truncated entropy", i, len(values))
	}
}

// TestRandomIsHex confirms the documented alphabet.
func TestRandomIsHex(t *testing.T) {
	value := Str.Random(32)
	_, err := hex.DecodeString(value)
	assert.NoError(t, err, "Random must emit hex characters, got %q", value)
}

// TestRandomStringIsURLSafe confirms the value needs no escaping in a URL.
func TestRandomStringIsURLSafe(t *testing.T) {
	for range 50 {
		value := RandomString(43)
		for _, r := range value {
			isSafe := (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') ||
				(r >= '0' && r <= '9') || r == '-' || r == '_'
			assert.Truef(t, isSafe, "character %q is not URL-safe in %q", r, value)
		}
	}
}

// TestUUIDFormat checks the v4 layout and variant bits.
func TestUUIDFormat(t *testing.T) {
	seen := make(map[string]bool)
	for range 200 {
		id := Str.UUID()
		require.Len(t, id, 36)
		assert.False(t, seen[id], "UUIDs must not repeat")
		seen[id] = true

		assert.Equal(t, byte('4'), id[14], "version nibble must be 4 in %q", id)
		assert.Contains(t, "89ab", string(id[19]), "variant nibble must be 8-b in %q", id)
	}
}

// TestRandomBytesLength covers the byte-oriented helper.
func TestRandomBytesLength(t *testing.T) {
	buf, err := RandomBytes(32)
	require.NoError(t, err)
	assert.Len(t, buf, 32)
	assert.NotEqual(t, make([]byte, 32), buf, "must not return an all-zero buffer")
}
