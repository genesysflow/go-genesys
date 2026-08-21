package urlsign_test

import (
	"strings"
	"testing"
	"time"

	"github.com/genesysflow/go-genesys/urlsign"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSignAndVerify(t *testing.T) {
	signer := urlsign.New([]byte("test-key-32-bytes-long-enough!!!"))

	signed, err := signer.Sign("https://example.com/unsubscribe?user=42")
	require.NoError(t, err)
	assert.Contains(t, signed, "signature=")
	assert.True(t, signer.Verify(signed))

	// Tampering with any parameter invalidates the signature.
	assert.False(t, signer.Verify(strings.Replace(signed, "user=42", "user=43", 1)))

	// Unsigned URLs fail verification.
	assert.False(t, signer.Verify("https://example.com/unsubscribe?user=42"))

	// A different key fails verification.
	other := urlsign.New([]byte("another-key-that-is-different!!!"))
	assert.False(t, other.Verify(signed))
}

func TestTemporarySignedURL(t *testing.T) {
	signer := urlsign.New([]byte("test-key"))

	signed, err := signer.SignTemporary("https://example.com/download?file=1", time.Hour)
	require.NoError(t, err)
	assert.Contains(t, signed, "expires=")
	assert.True(t, signer.Verify(signed))

	expired, err := signer.SignTemporary("https://example.com/download?file=1", -time.Hour)
	require.NoError(t, err)
	assert.False(t, signer.Verify(expired))

	// Stripping the expires parameter breaks the signature.
	stripped := strings.Split(signed, "expires=")[0] + "signature=" + strings.Split(signed, "signature=")[1]
	assert.False(t, signer.Verify(stripped))
}

func TestQueryOrderIndependent(t *testing.T) {
	signer := urlsign.New([]byte("k"))
	signed, err := signer.Sign("/path?b=2&a=1")
	require.NoError(t, err)
	assert.True(t, signer.Verify(signed))
}

func TestDoubleSignRejected(t *testing.T) {
	signer := urlsign.New([]byte("k"))
	signed, err := signer.Sign("/path?a=1")
	require.NoError(t, err)
	_, err = signer.Sign(signed)
	assert.Error(t, err)
}
