package mail_test

import (
	"testing"

	"github.com/genesysflow/go-genesys/mail"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Addresses with CRLF must be rejected, and header values must never
// carry newlines into the rendered message (header injection).
func TestBytesRejectsAddressInjection(t *testing.T) {
	_, err := mail.NewMessage().
		From("me@a.com").
		To("victim@b.com\r\nBcc: attacker@evil.com").
		Subject("hi").Text("b").
		Bytes()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "newline")
}

func TestBytesSanitisesCustomHeaders(t *testing.T) {
	raw, err := mail.NewMessage().
		From("me@a.com").To("you@b.com").
		Subject("hi").Text("b").
		Header("X-Custom", "value\r\nBcc: attacker@evil.com").
		Bytes()
	require.NoError(t, err)
	// The CRLF was stripped: the smuggled text stays inside the
	// X-Custom header's line instead of becoming its own Bcc header.
	assert.NotContains(t, string(raw), "\r\nBcc: attacker@evil.com")
	assert.Contains(t, string(raw), "X-Custom: valueBcc: attacker@evil.com")
}
