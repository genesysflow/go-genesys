package filesystem

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Default permissions for files and directories created by this driver.
//
// These are deliberately owner-only. A storage root routinely holds user
// uploads, cached credentials and generated exports; world-readable bits mean
// every local account on the host can read them. A disk that genuinely serves
// public assets through another process should widen them explicitly via the
// "permissions" config, rather than every disk being permissive by default.
const (
	defaultFilePermission os.FileMode = 0o600
	defaultDirPermission  os.FileMode = 0o700
)

// Local is the local filesystem driver.
type Local struct {
	root     string
	url      string
	filePerm os.FileMode
	dirPerm  os.FileMode
}

// NewLocal creates a new local filesystem instance.
//
// Recognised config keys: "root" (required), "url", and optionally
// "permissions" as a map with "file" and "dir" octal modes.
func NewLocal(config map[string]any) (*Local, error) {
	root, ok := config["root"].(string)
	if !ok {
		return nil, fmt.Errorf("filesystem: root not defined for local driver")
	}

	// Get absolute path for root to ensure consistent path validation
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("filesystem: failed to resolve root path: %w", err)
	}

	// Resolve the root through any symlinks once, up front, so that the
	// containment check below compares two fully-resolved paths. If the root
	// does not exist yet the absolute path stands in until it does.
	if resolvedRoot, err := filepath.EvalSymlinks(absRoot); err == nil {
		absRoot = resolvedRoot
	}

	url, _ := config["url"].(string)

	local := &Local{
		root:     absRoot,
		url:      url,
		filePerm: defaultFilePermission,
		dirPerm:  defaultDirPermission,
	}

	if permissions, ok := config["permissions"].(map[string]any); ok {
		if mode, ok := toFileMode(permissions["file"]); ok {
			local.filePerm = mode
		}
		if mode, ok := toFileMode(permissions["dir"]); ok {
			local.dirPerm = mode
		}
	}

	return local, nil
}

// toFileMode converts a config value to an os.FileMode.
func toFileMode(value any) (os.FileMode, bool) {
	switch v := value.(type) {
	case int:
		return os.FileMode(v), true
	case int64:
		return os.FileMode(v), true
	case os.FileMode:
		return v, true
	default:
		return 0, false
	}
}

// path resolves a caller-supplied path against the root and refuses anything
// that escapes it.
//
// Lexical cleaning alone is not sufficient: a symlink inside the root that
// points outside it survives filepath.Clean untouched, so "link/secret" would
// pass a prefix check while reading an arbitrary file. Every existing path
// component is therefore resolved before the containment test.
func (l *Local) path(path string) (string, error) {
	// Clean the path to remove any ".." or "." components
	cleanPath := filepath.Clean(path)

	// Join with root and get absolute path
	fullPath := filepath.Join(l.root, cleanPath)
	absPath, err := filepath.Abs(fullPath)
	if err != nil {
		return "", fmt.Errorf("filesystem: failed to resolve path: %w", err)
	}

	if err := l.checkContained(absPath); err != nil {
		return "", err
	}

	return absPath, nil
}

// checkContained verifies that absPath resolves to a location beneath the
// root, following symlinks on whatever prefix of the path already exists.
func (l *Local) checkContained(absPath string) error {
	// Resolve the deepest existing ancestor. A create targets a path that does
	// not exist yet, but its parent directory does, and that is where a
	// symlink would be planted.
	probe := absPath
	var trailing []string
	for {
		resolved, err := filepath.EvalSymlinks(probe)
		if err == nil {
			candidate := filepath.Join(append([]string{resolved}, trailing...)...)
			if !isWithin(candidate, l.root) {
				return fmt.Errorf("filesystem: path traversal detected: %s", absPath)
			}
			return nil
		}

		if !os.IsNotExist(err) {
			return fmt.Errorf("filesystem: failed to resolve path: %w", err)
		}

		parent := filepath.Dir(probe)
		if parent == probe {
			// Reached the filesystem root without finding anything that
			// exists; fall back to the lexical check.
			break
		}
		trailing = append([]string{filepath.Base(probe)}, trailing...)
		probe = parent
	}

	if !isWithin(absPath, l.root) {
		return fmt.Errorf("filesystem: path traversal detected: %s", absPath)
	}

	return nil
}

// isWithin reports whether path sits strictly beneath root. The root itself is
// excluded so that whole-store operations cannot be triggered with an empty
// path.
func isWithin(path, root string) bool {
	return strings.HasPrefix(path, root+string(filepath.Separator))
}

func (l *Local) Exists(ctx context.Context, path string) bool {
	if err := ctx.Err(); err != nil {
		return false
	}
	fullPath, err := l.path(path)
	if err != nil {
		return false
	}
	_, err = os.Stat(fullPath)
	return !os.IsNotExist(err)
}

func (l *Local) Get(ctx context.Context, path string) (string, error) {
	b, err := l.GetBytes(ctx, path)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func (l *Local) GetBytes(ctx context.Context, path string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	fullPath, err := l.path(path)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(fullPath)
}

func (l *Local) Put(ctx context.Context, path string, contents string) error {
	return l.PutBytes(ctx, path, []byte(contents))
}

func (l *Local) PutBytes(ctx context.Context, path string, contents []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	fullPath, err := l.path(path)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(fullPath), l.dirPerm); err != nil {
		return err
	}
	return os.WriteFile(fullPath, contents, l.filePerm)
}

func (l *Local) PutStream(ctx context.Context, path string, contents io.Reader) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	fullPath, err := l.path(path)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(fullPath), l.dirPerm); err != nil {
		return err
	}

	f, err := os.OpenFile(fullPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, l.filePerm)
	if err != nil {
		return err
	}
	defer f.Close()

	// Watch for context cancellation
	done := make(chan error, 1)
	go func() {
		_, err := io.Copy(f, contents)
		done <- err
	}()

	select {
	case <-ctx.Done():
		// Close file before removing to avoid resource leaks and file locking issues
		f.Close()
		os.Remove(fullPath)
		return ctx.Err()
	case err := <-done:
		return err
	}
}

func (l *Local) Delete(ctx context.Context, path string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	fullPath, err := l.path(path)
	if err != nil {
		return err
	}
	return os.Remove(fullPath)
}

func (l *Local) Copy(ctx context.Context, from, to string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	sourcePath, err := l.path(from)
	if err != nil {
		return err
	}
	destPath, err := l.path(to)
	if err != nil {
		return err
	}

	// Check if source exists
	if !l.Exists(ctx, from) {
		return os.ErrNotExist
	}

	// Create destination directory
	if err := os.MkdirAll(filepath.Dir(destPath), l.dirPerm); err != nil {
		return err
	}

	// Open source
	source, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer source.Close()

	// Create destination
	dest, err := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, l.filePerm)
	if err != nil {
		return err
	}
	defer dest.Close()

	// Watch for context cancellation
	done := make(chan error, 1)
	go func() {
		_, err := io.Copy(dest, source)
		done <- err
	}()

	select {
	case <-ctx.Done():
		// Close files before removing to avoid resource leaks and file locking issues
		source.Close()
		dest.Close()
		os.Remove(destPath)
		return ctx.Err()
	case err := <-done:
		return err
	}
}

func (l *Local) Move(ctx context.Context, from, to string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	sourcePath, err := l.path(from)
	if err != nil {
		return err
	}
	destPath, err := l.path(to)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(destPath), l.dirPerm); err != nil {
		return err
	}

	return os.Rename(sourcePath, destPath)
}

func (l *Local) Size(ctx context.Context, path string) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	fullPath, err := l.path(path)
	if err != nil {
		return 0, err
	}
	info, err := os.Stat(fullPath)
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

func (l *Local) LastModified(ctx context.Context, path string) (time.Time, error) {
	if err := ctx.Err(); err != nil {
		return time.Time{}, err
	}
	fullPath, err := l.path(path)
	if err != nil {
		return time.Time{}, err
	}
	info, err := os.Stat(fullPath)
	if err != nil {
		return time.Time{}, err
	}
	return info.ModTime(), nil
}

func (l *Local) MakeDirectory(ctx context.Context, path string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	fullPath, err := l.path(path)
	if err != nil {
		return err
	}
	return os.MkdirAll(fullPath, l.dirPerm)
}

func (l *Local) DeleteDirectory(ctx context.Context, path string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	fullPath, err := l.path(path)
	if err != nil {
		return err
	}
	return os.RemoveAll(fullPath)
}

func (l *Local) Url(path string) string {
	return strings.TrimRight(l.url, "/") + "/" + strings.TrimLeft(path, "/")
}
