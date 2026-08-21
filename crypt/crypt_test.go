package crypt_test

import (
	"strings"
	"testing"

	"github.com/genesysflow/go-genesys/crypt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEncryptDecryptRoundTrip(t *testing.T) {
	encrypter, err := crypt.NewFromString(crypt.GenerateKey())
	require.NoError(t, err)

	payload, err := encrypter.EncryptString("secret value")
	require.NoError(t, err)
	assert.NotContains(t, payload, "secret")

	plaintext, err := encrypter.DecryptString(payload)
	require.NoError(t, err)
	assert.Equal(t, "secret value", plaintext)

	// Same plaintext encrypts to different payloads (random nonce).
	payload2, err := encrypter.EncryptString("secret value")
	require.NoError(t, err)
	assert.NotEqual(t, payload, payload2)
}

func TestTamperedPayloadRejected(t *testing.T) {
	encrypter, err := crypt.NewFromString(crypt.GenerateKey())
	require.NoError(t, err)

	payload, err := encrypter.EncryptString("data")
	require.NoError(t, err)

	// Flip a character in the payload.
	tampered := []byte(payload)
	if tampered[10] == 'A' {
		tampered[10] = 'B'
	} else {
		tampered[10] = 'A'
	}
	_, err = encrypter.DecryptString(string(tampered))
	assert.ErrorIs(t, err, crypt.ErrInvalidPayload)

	_, err = encrypter.DecryptString("not base64!!!")
	assert.ErrorIs(t, err, crypt.ErrInvalidPayload)

	_, err = encrypter.DecryptString("c2hvcnQ=")
	assert.ErrorIs(t, err, crypt.ErrInvalidPayload)
}

func TestWrongKeyRejected(t *testing.T) {
	a, err := crypt.NewFromString(crypt.GenerateKey())
	require.NoError(t, err)
	b, err := crypt.NewFromString(crypt.GenerateKey())
	require.NoError(t, err)

	payload, err := a.EncryptString("data")
	require.NoError(t, err)
	_, err = b.DecryptString(payload)
	assert.ErrorIs(t, err, crypt.ErrInvalidPayload)
}

func TestGenerateKeyFormat(t *testing.T) {
	key := crypt.GenerateKey()
	assert.True(t, strings.HasPrefix(key, "base64:"))

	raw, err := crypt.ParseKey(key)
	require.NoError(t, err)
	assert.Len(t, raw, 32)
}

func TestEmptyKeyRejected(t *testing.T) {
	_, err := crypt.NewFromString("")
	assert.Error(t, err)
}
