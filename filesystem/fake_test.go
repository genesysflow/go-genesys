package filesystem_test

import (
	"context"
	"strings"
	"testing"

	"github.com/genesysflow/go-genesys/filesystem"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFakeDiskRoundTrip(t *testing.T) {
	ctx := context.Background()
	disk := filesystem.NewFakeDisk()

	require.NoError(t, disk.Put(ctx, "avatars/1.png", "png-bytes"))
	disk.AssertExists(t, "avatars/1.png")

	got, err := disk.Get(ctx, "avatars/1.png")
	require.NoError(t, err)
	assert.Equal(t, "png-bytes", got)

	size, err := disk.Size(ctx, "avatars/1.png")
	require.NoError(t, err)
	assert.EqualValues(t, 9, size)

	require.NoError(t, disk.PutStream(ctx, "docs/a.txt", strings.NewReader("stream")))
	require.NoError(t, disk.Copy(ctx, "docs/a.txt", "docs/b.txt"))
	require.NoError(t, disk.Move(ctx, "docs/b.txt", "docs/c.txt"))
	disk.AssertMissing(t, "docs/b.txt")
	disk.AssertExists(t, "docs/c.txt")

	require.NoError(t, disk.DeleteDirectory(ctx, "docs"))
	disk.AssertMissing(t, "docs/a.txt")
	disk.AssertMissing(t, "docs/c.txt")
	disk.AssertExists(t, "avatars/1.png")

	assert.Equal(t, "/storage/avatars/1.png", disk.Url("avatars/1.png"))
}
