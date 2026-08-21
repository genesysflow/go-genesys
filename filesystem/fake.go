package filesystem

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
)

// FakeDisk is an in-memory filesystem for tests - Laravel's
// Storage::fake(). It implements contracts.Filesystem, so anything that
// takes a disk accepts it, and it carries assertions:
//
//	disk := filesystem.NewFakeDisk()
//	service := NewAvatarService(disk)
//	service.Store(ctx, user, upload)
//	disk.AssertExists(t, "avatars/1.png")
type FakeDisk struct {
	mu      sync.RWMutex
	files   map[string]fakeFile
	baseURL string
}

type fakeFile struct {
	content []byte
	modTime time.Time
}

// NewFakeDisk creates an empty fake disk.
func NewFakeDisk() *FakeDisk {
	return &FakeDisk{files: make(map[string]fakeFile), baseURL: "/storage"}
}

func normalizeFakePath(path string) string {
	return strings.TrimPrefix(path, "/")
}

// Exists checks if a file exists.
func (d *FakeDisk) Exists(_ context.Context, path string) bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	_, ok := d.files[normalizeFakePath(path)]
	return ok
}

// Get retrieves the contents of a file.
func (d *FakeDisk) Get(ctx context.Context, path string) (string, error) {
	raw, err := d.GetBytes(ctx, path)
	return string(raw), err
}

// GetBytes retrieves the contents of a file as bytes.
func (d *FakeDisk) GetBytes(_ context.Context, path string) ([]byte, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	file, ok := d.files[normalizeFakePath(path)]
	if !ok {
		return nil, fmt.Errorf("filesystem: file %q does not exist", path)
	}
	return append([]byte(nil), file.content...), nil
}

// Put stores a file.
func (d *FakeDisk) Put(ctx context.Context, path string, contents string) error {
	return d.PutBytes(ctx, path, []byte(contents))
}

// PutBytes stores a file with byte content.
func (d *FakeDisk) PutBytes(_ context.Context, path string, contents []byte) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.files[normalizeFakePath(path)] = fakeFile{
		content: append([]byte(nil), contents...),
		modTime: time.Now(),
	}
	return nil
}

// PutStream stores a file from a reader.
func (d *FakeDisk) PutStream(ctx context.Context, path string, contents io.Reader) error {
	raw, err := io.ReadAll(contents)
	if err != nil {
		return err
	}
	return d.PutBytes(ctx, path, raw)
}

// Delete deletes a file.
func (d *FakeDisk) Delete(_ context.Context, path string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.files, normalizeFakePath(path))
	return nil
}

// Copy copies a file to a new location.
func (d *FakeDisk) Copy(ctx context.Context, from, to string) error {
	raw, err := d.GetBytes(ctx, from)
	if err != nil {
		return err
	}
	return d.PutBytes(ctx, to, raw)
}

// Move moves a file to a new location.
func (d *FakeDisk) Move(ctx context.Context, from, to string) error {
	if err := d.Copy(ctx, from, to); err != nil {
		return err
	}
	return d.Delete(ctx, from)
}

// Size gets the file size in bytes.
func (d *FakeDisk) Size(_ context.Context, path string) (int64, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	file, ok := d.files[normalizeFakePath(path)]
	if !ok {
		return 0, fmt.Errorf("filesystem: file %q does not exist", path)
	}
	return int64(len(file.content)), nil
}

// LastModified gets the file's last modified time.
func (d *FakeDisk) LastModified(_ context.Context, path string) (time.Time, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	file, ok := d.files[normalizeFakePath(path)]
	if !ok {
		return time.Time{}, fmt.Errorf("filesystem: file %q does not exist", path)
	}
	return file.modTime, nil
}

// MakeDirectory creates a directory (a no-op: the fake is flat).
func (d *FakeDisk) MakeDirectory(context.Context, string) error { return nil }

// DeleteDirectory deletes every file under the directory prefix.
func (d *FakeDisk) DeleteDirectory(_ context.Context, path string) error {
	prefix := strings.TrimSuffix(normalizeFakePath(path), "/") + "/"
	d.mu.Lock()
	defer d.mu.Unlock()
	for name := range d.files {
		if strings.HasPrefix(name, prefix) {
			delete(d.files, name)
		}
	}
	return nil
}

// Url returns the public URL for the file.
func (d *FakeDisk) Url(path string) string {
	return d.baseURL + "/" + normalizeFakePath(path)
}

// Files lists every stored path, for debugging.
func (d *FakeDisk) Files() []string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	paths := make([]string, 0, len(d.files))
	for name := range d.files {
		paths = append(paths, name)
	}
	return paths
}

// testingT is the minimal testing surface the assertions need.
type testingT interface {
	Helper()
	Errorf(format string, args ...any)
}

// AssertExists fails the test unless the file was stored.
func (d *FakeDisk) AssertExists(t testingT, path string) {
	t.Helper()
	if !d.Exists(context.Background(), path) {
		t.Errorf("filesystem: expected %q to exist; stored files: %v", path, d.Files())
	}
}

// AssertMissing fails the test when the file was stored.
func (d *FakeDisk) AssertMissing(t testingT, path string) {
	t.Helper()
	if d.Exists(context.Background(), path) {
		t.Errorf("filesystem: expected %q to be missing", path)
	}
}
