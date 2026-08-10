package filesystem

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newSandbox builds a storage root with a secret planted outside it.
func newSandbox(t *testing.T) (root, outside string) {
	t.Helper()

	base := t.TempDir()
	root = filepath.Join(base, "storage")
	outside = filepath.Join(base, "outside")

	require.NoError(t, os.MkdirAll(root, 0o700))
	require.NoError(t, os.MkdirAll(outside, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("TOP SECRET"), 0o600))

	return root, outside
}

// TestLocalRejectsLexicalTraversal covers plain "../" escapes.
func TestLocalRejectsLexicalTraversal(t *testing.T) {
	root, _ := newSandbox(t)

	fs, err := NewLocal(map[string]any{"root": root})
	require.NoError(t, err)

	for _, path := range []string{
		"../outside/secret.txt",
		"../../etc/passwd",
		"a/../../outside/secret.txt",
		"/../outside/secret.txt",
	} {
		t.Run(path, func(t *testing.T) {
			_, err := fs.Get(context.Background(), path)
			assert.Error(t, err, "traversal must be refused")
		})
	}
}

// TestLocalRejectsSymlinkEscape is the regression test for a containment
// bypass: filepath.Clean leaves a symlink component untouched, so a purely
// lexical prefix check accepts "link/secret.txt" while it resolves to an
// arbitrary location outside the root.
func TestLocalRejectsSymlinkEscape(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires elevation on Windows")
	}

	root, outside := newSandbox(t)
	require.NoError(t, os.Symlink(outside, filepath.Join(root, "link")))

	fs, err := NewLocal(map[string]any{"root": root})
	require.NoError(t, err)
	ctx := context.Background()

	t.Run("read through symlink", func(t *testing.T) {
		_, err := fs.Get(ctx, "link/secret.txt")
		assert.Error(t, err)
	})

	t.Run("write through symlink", func(t *testing.T) {
		err := fs.Put(ctx, "link/planted.txt", "payload")
		assert.Error(t, err)
		assert.NoFileExists(t, filepath.Join(outside, "planted.txt"),
			"a refused write must not have reached the target")
	})

	t.Run("delete through symlink", func(t *testing.T) {
		err := fs.Delete(ctx, "link/secret.txt")
		assert.Error(t, err)
		assert.FileExists(t, filepath.Join(outside, "secret.txt"),
			"a refused delete must leave the file intact")
	})

	t.Run("stat through symlink", func(t *testing.T) {
		_, err := fs.Size(ctx, "link/secret.txt")
		assert.Error(t, err)
	})

	t.Run("copy out through symlink", func(t *testing.T) {
		require.NoError(t, fs.Put(ctx, "inside.txt", "data"))
		err := fs.Copy(ctx, "inside.txt", "link/exfiltrated.txt")
		assert.Error(t, err)
		assert.NoFileExists(t, filepath.Join(outside, "exfiltrated.txt"))
	})
}

// TestLocalSymlinkToFileEscape covers a symlink to a file rather than a
// directory.
func TestLocalSymlinkToFileEscape(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires elevation on Windows")
	}

	root, outside := newSandbox(t)
	require.NoError(t, os.Symlink(filepath.Join(outside, "secret.txt"), filepath.Join(root, "secret-link")))

	fs, err := NewLocal(map[string]any{"root": root})
	require.NoError(t, err)

	_, err = fs.Get(context.Background(), "secret-link")
	assert.Error(t, err)
}

// TestLocalNormalOperationsStillWork guards against the containment check
// being so strict it breaks ordinary use.
func TestLocalNormalOperationsStillWork(t *testing.T) {
	root, _ := newSandbox(t)

	fs, err := NewLocal(map[string]any{"root": root})
	require.NoError(t, err)
	ctx := context.Background()

	require.NoError(t, fs.Put(ctx, "nested/dir/file.txt", "hello"))

	got, err := fs.Get(ctx, "nested/dir/file.txt")
	require.NoError(t, err)
	assert.Equal(t, "hello", got)

	assert.True(t, fs.Exists(ctx, "nested/dir/file.txt"))

	require.NoError(t, fs.Copy(ctx, "nested/dir/file.txt", "copy.txt"))
	copied, err := fs.Get(ctx, "copy.txt")
	require.NoError(t, err)
	assert.Equal(t, "hello", copied)

	require.NoError(t, fs.Move(ctx, "copy.txt", "moved.txt"))
	assert.True(t, fs.Exists(ctx, "moved.txt"))
	assert.False(t, fs.Exists(ctx, "copy.txt"))

	require.NoError(t, fs.Delete(ctx, "moved.txt"))
	assert.False(t, fs.Exists(ctx, "moved.txt"))
}

// TestLocalDefaultPermissionsAreOwnerOnly: a storage root holds uploads and
// generated exports, so it must not be world-readable by default.
func TestLocalDefaultPermissionsAreOwnerOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits are not meaningful on Windows")
	}

	root, _ := newSandbox(t)

	fs, err := NewLocal(map[string]any{"root": root})
	require.NoError(t, err)
	require.NoError(t, fs.Put(context.Background(), "sub/file.txt", "data"))

	fileInfo, err := os.Stat(filepath.Join(root, "sub/file.txt"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), fileInfo.Mode().Perm(),
		"files must not be readable by other local accounts")

	dirInfo, err := os.Stat(filepath.Join(root, "sub"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o700), dirInfo.Mode().Perm())
}

// TestLocalPermissionsAreConfigurable: a disk whose files are served publicly
// by another process can widen them deliberately.
func TestLocalPermissionsAreConfigurable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits are not meaningful on Windows")
	}

	root, _ := newSandbox(t)

	fs, err := NewLocal(map[string]any{
		"root":        root,
		"permissions": map[string]any{"file": 0o644, "dir": 0o755},
	})
	require.NoError(t, err)
	require.NoError(t, fs.Put(context.Background(), "pub/file.txt", "data"))

	fileInfo, err := os.Stat(filepath.Join(root, "pub/file.txt"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o644), fileInfo.Mode().Perm())
}
