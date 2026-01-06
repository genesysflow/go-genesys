package contracts

import (
	"context"
	"io"
	"time"
)

// Filesystem defines the interface for filesystem operations.
type Filesystem interface {
	// Exists checks if a file exists.
	Exists(ctx context.Context, path string) bool

	// Get retrieves the contents of a file.
	Get(ctx context.Context, path string) (string, error)

	// GetBytes retrieves the contents of a file as bytes.
	GetBytes(ctx context.Context, path string) ([]byte, error)

	// Put stores a file.
	Put(ctx context.Context, path string, contents string) error

	// PutBytes stores a file with byte content.
	PutBytes(ctx context.Context, path string, contents []byte) error

	// PutStream stores a file from a reader.
	PutStream(ctx context.Context, path string, contents io.Reader) error

	// Delete deletes a file.
	Delete(ctx context.Context, path string) error

	// Copy copies a file to a new location.
	Copy(ctx context.Context, from, to string) error

	// Move moves a file to a new location.
	Move(ctx context.Context, from, to string) error

	// Size gets the file size in bytes.
	Size(ctx context.Context, path string) (int64, error)

	// LastModified gets the file's last modified time.
	LastModified(ctx context.Context, path string) (time.Time, error)

	// MakeDirectory creates a directory.
	MakeDirectory(ctx context.Context, path string) error

	// DeleteDirectory deletes a directory.
	DeleteDirectory(ctx context.Context, path string) error

	// Url returns the public URL for the file.
	Url(path string) string
}

// FilesystemFactory defines the interface for creating filesystem instances.
type FilesystemFactory interface {
	// Disk gets a filesystem instance by name.
	Disk(name ...string) Filesystem
}
