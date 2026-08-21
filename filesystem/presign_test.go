package filesystem_test

import (
	"context"
	"net/url"
	"testing"
	"time"

	"github.com/genesysflow/go-genesys/contracts"
	"github.com/genesysflow/go-genesys/facades/storage"
	"github.com/genesysflow/go-genesys/filesystem"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Presigning is pure local cryptography - no request reaches S3 - so it
// is fully testable offline.
func TestS3TemporaryURL(t *testing.T) {
	disk, err := filesystem.NewS3(map[string]any{
		"key":      "AKIAEXAMPLE",
		"secret":   "secret-key",
		"region":   "eu-central-1",
		"bucket":   "invoices",
		"endpoint": "https://s3.example.test",
	})
	require.NoError(t, err)

	signed, err := disk.TemporaryURL(context.Background(), "2026/invoice-42.pdf", 15*time.Minute)
	require.NoError(t, err)

	parsed, err := url.Parse(signed)
	require.NoError(t, err)
	assert.Contains(t, parsed.Host, "s3.example.test")
	assert.Contains(t, parsed.Path, "invoice-42.pdf")

	params := parsed.Query()
	assert.Equal(t, "900", params.Get("X-Amz-Expires"), "TTL encoded in seconds")
	assert.NotEmpty(t, params.Get("X-Amz-Signature"))
	assert.Contains(t, params.Get("X-Amz-Credential"), "AKIAEXAMPLE")
	assert.Equal(t, "AWS4-HMAC-SHA256", params.Get("X-Amz-Algorithm"))

	// Different TTLs and keys produce different signatures.
	other, err := disk.TemporaryURL(context.Background(), "2026/invoice-43.pdf", 15*time.Minute)
	require.NoError(t, err)
	assert.NotEqual(t, signed, other)
}

func TestLocalDiskCannotMintTemporaryURLs(t *testing.T) {
	local, err := filesystem.NewLocal(map[string]any{"root": t.TempDir()})
	require.NoError(t, err)

	_, supported := any(local).(filesystem.TemporaryURLProvider)
	assert.False(t, supported, "local disks must not claim presigning support")

	storage.SetInstance(&fakeFactory{disk: local})
	t.Cleanup(func() { storage.SetInstance(nil) })
	_, err = storage.TemporaryURL(context.Background(), "file.txt", time.Minute)
	assert.ErrorContains(t, err, "does not support temporary URLs")
}

type fakeFactory struct{ disk contracts.Filesystem }

func (f *fakeFactory) Disk(name ...string) contracts.Filesystem { return f.disk }
