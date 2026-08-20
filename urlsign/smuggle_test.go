package urlsign_test

import (
	"net/url"
	"testing"

	"github.com/genesysflow/go-genesys/urlsign"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A signed value containing "&" / "=" must not be re-partitionable into
// different parameters that carry the same signature (parameter
// smuggling through ambiguous canonicalization).
func TestSignatureRejectsRepartitionedQuery(t *testing.T) {
	signer := urlsign.New([]byte("0123456789abcdef0123456789abcdef"))

	// The app legitimately signs a URL whose value embeds delimiters.
	signed, err := signer.Sign("https://app.test/grant?data=" + url.QueryEscape("x&role=admin"))
	require.NoError(t, err)
	assert.True(t, signer.Verify(signed), "the genuine URL verifies")

	// An attacker re-partitions the same bytes into separate parameters,
	// hoping the canonical string (and HMAC) comes out identical.
	parsed, err := url.Parse(signed)
	require.NoError(t, err)
	q := parsed.Query()
	sig := q.Get("signature")

	forged := "https://app.test/grant?data=x&role=admin&signature=" + sig
	assert.False(t, signer.Verify(forged), "smuggled parameters must not verify")
}
