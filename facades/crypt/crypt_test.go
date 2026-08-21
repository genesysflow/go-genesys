package crypt_test

import (
	"testing"

	basecrypt "github.com/genesysflow/go-genesys/crypt"
	"github.com/genesysflow/go-genesys/facades/crypt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFacadeRoundTrip(t *testing.T) {
	encrypter, err := basecrypt.NewFromString(basecrypt.GenerateKey())
	require.NoError(t, err)
	crypt.SetInstance(encrypter)
	t.Cleanup(func() { crypt.SetInstance(nil) })

	payload, err := crypt.EncryptString("secret")
	require.NoError(t, err)
	plain, err := crypt.DecryptString(payload)
	require.NoError(t, err)
	assert.Equal(t, "secret", plain)

	raw, err := crypt.Encrypt([]byte{1, 2, 3})
	require.NoError(t, err)
	bytes, err := crypt.Decrypt(raw)
	require.NoError(t, err)
	assert.Equal(t, []byte{1, 2, 3}, bytes)

	assert.NotNil(t, crypt.GetInstance())
}

func TestFacadePanicsWithoutInstance(t *testing.T) {
	crypt.SetInstance(nil)
	assert.Panics(t, func() { crypt.EncryptString("x") })
}
