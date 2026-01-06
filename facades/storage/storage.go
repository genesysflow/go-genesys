// Package storage provides a static facade for filesystem operations.
package storage

import (
	"context"
	"io"
	"sync"
	"time"

	"github.com/genesysflow/go-genesys/contracts"
)

var (
	instance contracts.FilesystemFactory
	mu       sync.RWMutex
)

// SetInstance sets the filesystem factory instance.
func SetInstance(factory contracts.FilesystemFactory) {
	mu.Lock()
	defer mu.Unlock()
	instance = factory
}

// Disk returns a filesystem instance by name.
func Disk(name ...string) contracts.Filesystem {
	mu.RLock()
	defer mu.RUnlock()
	if instance == nil {
		return nil
	}
	return instance.Disk(name...)
}

// Exists checks if a file exists on the default disk.
func Exists(ctx context.Context, path string) bool {
	return Disk().Exists(ctx, path)
}

// Get retrieves the contents of a file from the default disk.
func Get(ctx context.Context, path string) (string, error) {
	return Disk().Get(ctx, path)
}

// GetBytes retrieves the contents of a file as bytes from the default disk.
func GetBytes(ctx context.Context, path string) ([]byte, error) {
	return Disk().GetBytes(ctx, path)
}

// Put stores a file on the default disk.
func Put(ctx context.Context, path string, contents string) error {
	return Disk().Put(ctx, path, contents)
}

// PutBytes stores a file with byte content on the default disk.
func PutBytes(ctx context.Context, path string, contents []byte) error {
	return Disk().PutBytes(ctx, path, contents)
}

// PutStream stores a file from a reader on the default disk.
func PutStream(ctx context.Context, path string, contents io.Reader) error {
	return Disk().PutStream(ctx, path, contents)
}

// Delete deletes a file from the default disk.
func Delete(ctx context.Context, path string) error {
	return Disk().Delete(ctx, path)
}

// Copy copies a file to a new location on the default disk.
func Copy(ctx context.Context, from, to string) error {
	return Disk().Copy(ctx, from, to)
}

// Move moves a file to a new location on the default disk.
func Move(ctx context.Context, from, to string) error {
	return Disk().Move(ctx, from, to)
}

// Size gets the file size in bytes from the default disk.
func Size(ctx context.Context, path string) (int64, error) {
	return Disk().Size(ctx, path)
}

// LastModified gets the file's last modified time from the default disk.
func LastModified(ctx context.Context, path string) (time.Time, error) {
	return Disk().LastModified(ctx, path)
}

// MakeDirectory creates a directory on the default disk.
func MakeDirectory(ctx context.Context, path string) error {
	return Disk().MakeDirectory(ctx, path)
}

// DeleteDirectory deletes a directory on the default disk.
func DeleteDirectory(ctx context.Context, path string) error {
	return Disk().DeleteDirectory(ctx, path)
}

// Url returns the public URL for the file from the default disk.
func Url(path string) string {
	d := Disk()
	if d == nil {
		return ""
	}
	return d.Url(path)
}
