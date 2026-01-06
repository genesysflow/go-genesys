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

// Local is the local filesystem driver.
type Local struct {
	root string
	url  string
}

// NewLocal creates a new local filesystem instance.
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

	url, _ := config["url"].(string)

	return &Local{
		root: absRoot,
		url:  url,
	}, nil
}

func (l *Local) path(path string) (string, error) {
	// Clean the path to remove any ".." or "." components
	cleanPath := filepath.Clean(path)

	// Join with root and get absolute path
	fullPath := filepath.Join(l.root, cleanPath)
	absPath, err := filepath.Abs(fullPath)
	if err != nil {
		return "", fmt.Errorf("filesystem: failed to resolve path: %w", err)
	}

	// Ensure the resolved path is within the root directory (not the root itself)
	// This prevents operations on the root directory which could be dangerous
	if !strings.HasPrefix(absPath, l.root+string(filepath.Separator)) {
		return "", fmt.Errorf("filesystem: path traversal detected: %s", path)
	}

	return absPath, nil
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
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		return err
	}
	return os.WriteFile(fullPath, contents, 0644)
}

func (l *Local) PutStream(ctx context.Context, path string, contents io.Reader) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	fullPath, err := l.path(path)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		return err
	}

	f, err := os.Create(fullPath)
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
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return err
	}

	// Open source
	source, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer source.Close()

	// Create destination
	dest, err := os.Create(destPath)
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

	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
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
	return os.MkdirAll(fullPath, 0755)
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
