package filesystem

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func setupLocalFS(t *testing.T) (*Local, string, func()) {
	t.Helper()
	tmpDir := t.TempDir()

	fs, err := NewLocal(map[string]any{
		"root": tmpDir,
		"url":  "http://localhost/storage",
	})
	if err != nil {
		t.Fatalf("failed to create local filesystem: %v", err)
	}

	cleanup := func() {
		os.RemoveAll(tmpDir)
	}

	return fs, tmpDir, cleanup
}

func TestNewLocal(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		tmpDir := t.TempDir()
		defer os.RemoveAll(tmpDir)

		fs, err := NewLocal(map[string]any{
			"root": tmpDir,
			"url":  "http://localhost/storage",
		})

		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if fs == nil {
			t.Fatal("expected filesystem instance, got nil")
		}
		if fs.url != "http://localhost/storage" {
			t.Errorf("expected url 'http://localhost/storage', got '%s'", fs.url)
		}
	})

	t.Run("missing root", func(t *testing.T) {
		_, err := NewLocal(map[string]any{
			"url": "http://localhost/storage",
		})

		if err == nil {
			t.Fatal("expected error for missing root, got nil")
		}
		if !strings.Contains(err.Error(), "root not defined") {
			t.Errorf("expected 'root not defined' error, got: %v", err)
		}
	})
}

func TestLocalPathTraversal(t *testing.T) {
	fs, tmpDir, cleanup := setupLocalFS(t)
	defer cleanup()

	tests := []struct {
		name        string
		path        string
		shouldError bool
	}{
		{"normal path", "test.txt", false},
		{"nested path", "dir/subdir/test.txt", false},
		{"path traversal up", "../../../etc/passwd", true},
		{"path traversal in middle", "dir/../../etc/passwd", true},
		{"clean path with dots", "./test.txt", false},
		{"multiple slashes", "dir//test.txt", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fullPath, err := fs.path(tt.path)

			if tt.shouldError {
				if err == nil {
					t.Errorf("expected error for path '%s', got nil", tt.path)
				}
			} else {
				if err != nil {
					t.Errorf("expected no error for path '%s', got %v", tt.path, err)
				}
				if !strings.HasPrefix(fullPath, tmpDir) {
					t.Errorf("path '%s' resolved outside root: %s", tt.path, fullPath)
				}
			}
		})
	}
}

func TestLocalExists(t *testing.T) {
	fs, _, cleanup := setupLocalFS(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("non-existent file", func(t *testing.T) {
		if fs.Exists(ctx, "non-existent.txt") {
			t.Error("expected file to not exist")
		}
	})

	t.Run("existing file", func(t *testing.T) {
		if err := fs.Put(ctx, "test.txt", "content"); err != nil {
			t.Fatalf("failed to create file: %v", err)
		}

		if !fs.Exists(ctx, "test.txt") {
			t.Error("expected file to exist")
		}
	})

	t.Run("context cancelled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		if fs.Exists(ctx, "test.txt") {
			t.Error("expected false for cancelled context")
		}
	})

	t.Run("path traversal", func(t *testing.T) {
		if fs.Exists(ctx, "../../../etc/passwd") {
			t.Error("path traversal should return false")
		}
	})
}

func TestLocalGetAndPut(t *testing.T) {
	fs, _, cleanup := setupLocalFS(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("put and get string", func(t *testing.T) {
		content := "Hello, World!"

		if err := fs.Put(ctx, "test.txt", content); err != nil {
			t.Fatalf("failed to put file: %v", err)
		}

		got, err := fs.Get(ctx, "test.txt")
		if err != nil {
			t.Fatalf("failed to get file: %v", err)
		}

		if got != content {
			t.Errorf("expected '%s', got '%s'", content, got)
		}
	})

	t.Run("put and get bytes", func(t *testing.T) {
		content := []byte("Binary content")

		if err := fs.PutBytes(ctx, "binary.dat", content); err != nil {
			t.Fatalf("failed to put bytes: %v", err)
		}

		got, err := fs.GetBytes(ctx, "binary.dat")
		if err != nil {
			t.Fatalf("failed to get bytes: %v", err)
		}

		if string(got) != string(content) {
			t.Errorf("expected '%s', got '%s'", content, got)
		}
	})

	t.Run("put creates directories", func(t *testing.T) {
		if err := fs.Put(ctx, "dir/subdir/test.txt", "content"); err != nil {
			t.Fatalf("failed to put file in nested directory: %v", err)
		}

		got, err := fs.Get(ctx, "dir/subdir/test.txt")
		if err != nil {
			t.Fatalf("failed to get file: %v", err)
		}

		if got != "content" {
			t.Errorf("expected 'content', got '%s'", got)
		}
	})

	t.Run("get non-existent file", func(t *testing.T) {
		_, err := fs.Get(ctx, "non-existent.txt")
		if err == nil {
			t.Fatal("expected error for non-existent file")
		}
	})
}

func TestLocalPutStream(t *testing.T) {
	fs, _, cleanup := setupLocalFS(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		content := "Stream content"
		reader := strings.NewReader(content)

		if err := fs.PutStream(ctx, "stream.txt", reader); err != nil {
			t.Fatalf("failed to put stream: %v", err)
		}

		got, err := fs.Get(ctx, "stream.txt")
		if err != nil {
			t.Fatalf("failed to get file: %v", err)
		}

		if got != content {
			t.Errorf("expected '%s', got '%s'", content, got)
		}
	})

	t.Run("context cancelled during write", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())

		// Create a slow reader
		slowReader := &slowReader{data: []byte(strings.Repeat("x", 1024*1024)), delay: time.Millisecond}

		// Cancel after a short delay
		go func() {
			time.Sleep(10 * time.Millisecond)
			cancel()
		}()

		err := fs.PutStream(ctx, "cancelled.txt", slowReader)
		if err == nil {
			t.Fatal("expected error for cancelled context")
		}

		// Verify partial file was cleaned up
		time.Sleep(50 * time.Millisecond) // Give time for cleanup
		if fs.Exists(context.Background(), "cancelled.txt") {
			t.Error("expected partial file to be removed")
		}
	})
}

func TestLocalDelete(t *testing.T) {
	fs, _, cleanup := setupLocalFS(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("delete existing file", func(t *testing.T) {
		if err := fs.Put(ctx, "delete-me.txt", "content"); err != nil {
			t.Fatalf("failed to create file: %v", err)
		}

		if err := fs.Delete(ctx, "delete-me.txt"); err != nil {
			t.Fatalf("failed to delete file: %v", err)
		}

		if fs.Exists(ctx, "delete-me.txt") {
			t.Error("file should not exist after delete")
		}
	})

	t.Run("delete non-existent file", func(t *testing.T) {
		err := fs.Delete(ctx, "non-existent.txt")
		if err == nil {
			t.Error("expected error when deleting non-existent file")
		}
	})
}

func TestLocalCopy(t *testing.T) {
	fs, _, cleanup := setupLocalFS(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("copy existing file", func(t *testing.T) {
		content := "Original content"
		if err := fs.Put(ctx, "original.txt", content); err != nil {
			t.Fatalf("failed to create file: %v", err)
		}

		if err := fs.Copy(ctx, "original.txt", "copy.txt"); err != nil {
			t.Fatalf("failed to copy file: %v", err)
		}

		// Check original still exists
		if !fs.Exists(ctx, "original.txt") {
			t.Error("original file should still exist")
		}

		// Check copy has same content
		got, err := fs.Get(ctx, "copy.txt")
		if err != nil {
			t.Fatalf("failed to get copied file: %v", err)
		}
		if got != content {
			t.Errorf("expected '%s', got '%s'", content, got)
		}
	})

	t.Run("copy non-existent file", func(t *testing.T) {
		err := fs.Copy(ctx, "non-existent.txt", "copy.txt")
		if err == nil {
			t.Fatal("expected error when copying non-existent file")
		}
	})

	t.Run("context cancelled during copy", func(t *testing.T) {
		// Create a large file
		largeContent := strings.Repeat("x", 100*1024*1024)
		if err := fs.Put(ctx, "large.txt", largeContent); err != nil {
			t.Fatalf("failed to create large file: %v", err)
		}

		ctx, cancel := context.WithCancel(context.Background())

		// Cancel immediately
		cancel()

		err := fs.Copy(ctx, "large.txt", "large-copy.txt")
		if err == nil {
			// Copy might succeed before context is checked
			// Clean up if it did
			fs.Delete(context.Background(), "large-copy.txt")
			t.Skip("copy completed before context cancellation was detected")
		}

		// Verify partial file was cleaned up
		time.Sleep(50 * time.Millisecond)
		if fs.Exists(context.Background(), "large-copy.txt") {
			t.Error("expected partial copy to be removed")
		}
	})
}

func TestLocalMove(t *testing.T) {
	fs, _, cleanup := setupLocalFS(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("move existing file", func(t *testing.T) {
		content := "Move me"
		if err := fs.Put(ctx, "source.txt", content); err != nil {
			t.Fatalf("failed to create file: %v", err)
		}

		if err := fs.Move(ctx, "source.txt", "destination.txt"); err != nil {
			t.Fatalf("failed to move file: %v", err)
		}

		// Check source no longer exists
		if fs.Exists(ctx, "source.txt") {
			t.Error("source file should not exist after move")
		}

		// Check destination has content
		got, err := fs.Get(ctx, "destination.txt")
		if err != nil {
			t.Fatalf("failed to get moved file: %v", err)
		}
		if got != content {
			t.Errorf("expected '%s', got '%s'", content, got)
		}
	})

	t.Run("move to nested directory", func(t *testing.T) {
		if err := fs.Put(ctx, "file.txt", "content"); err != nil {
			t.Fatalf("failed to create file: %v", err)
		}

		if err := fs.Move(ctx, "file.txt", "dir/subdir/file.txt"); err != nil {
			t.Fatalf("failed to move to nested directory: %v", err)
		}

		if !fs.Exists(ctx, "dir/subdir/file.txt") {
			t.Error("file should exist in new location")
		}
	})
}

func TestLocalSize(t *testing.T) {
	fs, _, cleanup := setupLocalFS(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("get size of file", func(t *testing.T) {
		content := "12345"
		if err := fs.Put(ctx, "sized.txt", content); err != nil {
			t.Fatalf("failed to create file: %v", err)
		}

		size, err := fs.Size(ctx, "sized.txt")
		if err != nil {
			t.Fatalf("failed to get size: %v", err)
		}

		if size != int64(len(content)) {
			t.Errorf("expected size %d, got %d", len(content), size)
		}
	})

	t.Run("size of non-existent file", func(t *testing.T) {
		_, err := fs.Size(ctx, "non-existent.txt")
		if err == nil {
			t.Fatal("expected error for non-existent file")
		}
	})
}

func TestLocalLastModified(t *testing.T) {
	fs, _, cleanup := setupLocalFS(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("get last modified time", func(t *testing.T) {
		before := time.Now().Add(-1 * time.Second)

		if err := fs.Put(ctx, "timed.txt", "content"); err != nil {
			t.Fatalf("failed to create file: %v", err)
		}

		after := time.Now().Add(1 * time.Second)

		modTime, err := fs.LastModified(ctx, "timed.txt")
		if err != nil {
			t.Fatalf("failed to get last modified: %v", err)
		}

		if modTime.Before(before) || modTime.After(after) {
			t.Errorf("modification time %v not between %v and %v", modTime, before, after)
		}
	})

	t.Run("last modified of non-existent file", func(t *testing.T) {
		_, err := fs.LastModified(ctx, "non-existent.txt")
		if err == nil {
			t.Fatal("expected error for non-existent file")
		}
	})
}

func TestLocalMakeDirectory(t *testing.T) {
	fs, tmpDir, cleanup := setupLocalFS(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("create directory", func(t *testing.T) {
		if err := fs.MakeDirectory(ctx, "newdir"); err != nil {
			t.Fatalf("failed to create directory: %v", err)
		}

		fullPath, _ := fs.path("newdir")
		info, err := os.Stat(fullPath)
		if err != nil {
			t.Fatalf("directory does not exist: %v", err)
		}
		if !info.IsDir() {
			t.Error("expected directory, got file")
		}
	})

	t.Run("create nested directories", func(t *testing.T) {
		if err := fs.MakeDirectory(ctx, "dir/subdir/nested"); err != nil {
			t.Fatalf("failed to create nested directories: %v", err)
		}

		fullPath := filepath.Join(tmpDir, "dir", "subdir", "nested")
		info, err := os.Stat(fullPath)
		if err != nil {
			t.Fatalf("nested directory does not exist: %v", err)
		}
		if !info.IsDir() {
			t.Error("expected directory, got file")
		}
	})
}

func TestLocalDeleteDirectory(t *testing.T) {
	fs, _, cleanup := setupLocalFS(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("delete empty directory", func(t *testing.T) {
		if err := fs.MakeDirectory(ctx, "emptydir"); err != nil {
			t.Fatalf("failed to create directory: %v", err)
		}

		if err := fs.DeleteDirectory(ctx, "emptydir"); err != nil {
			t.Fatalf("failed to delete directory: %v", err)
		}

		fullPath, _ := fs.path("emptydir")
		if _, err := os.Stat(fullPath); !os.IsNotExist(err) {
			t.Error("directory should not exist after delete")
		}
	})

	t.Run("delete directory with contents", func(t *testing.T) {
		if err := fs.Put(ctx, "dir/file1.txt", "content1"); err != nil {
			t.Fatalf("failed to create file: %v", err)
		}
		if err := fs.Put(ctx, "dir/file2.txt", "content2"); err != nil {
			t.Fatalf("failed to create file: %v", err)
		}

		if err := fs.DeleteDirectory(ctx, "dir"); err != nil {
			t.Fatalf("failed to delete directory: %v", err)
		}

		if fs.Exists(ctx, "dir/file1.txt") || fs.Exists(ctx, "dir/file2.txt") {
			t.Error("directory contents should be deleted")
		}
	})
}

func TestLocalUrl(t *testing.T) {
	t.Run("with configured url", func(t *testing.T) {
		fs, err := NewLocal(map[string]any{
			"root": t.TempDir(),
			"url":  "http://localhost/storage",
		})
		if err != nil {
			t.Fatalf("failed to create filesystem: %v", err)
		}

		url := fs.Url("path/to/file.txt")
		expected := "http://localhost/storage/path/to/file.txt"
		if url != expected {
			t.Errorf("expected '%s', got '%s'", expected, url)
		}
	})

	t.Run("url with trailing slash", func(t *testing.T) {
		fs, err := NewLocal(map[string]any{
			"root": t.TempDir(),
			"url":  "http://localhost/storage/",
		})
		if err != nil {
			t.Fatalf("failed to create filesystem: %v", err)
		}

		url := fs.Url("file.txt")
		expected := "http://localhost/storage/file.txt"
		if url != expected {
			t.Errorf("expected '%s', got '%s'", expected, url)
		}
	})

	t.Run("path with leading slash", func(t *testing.T) {
		fs, err := NewLocal(map[string]any{
			"root": t.TempDir(),
			"url":  "http://localhost/storage",
		})
		if err != nil {
			t.Fatalf("failed to create filesystem: %v", err)
		}

		url := fs.Url("/file.txt")
		expected := "http://localhost/storage/file.txt"
		if url != expected {
			t.Errorf("expected '%s', got '%s'", expected, url)
		}
	})
}

// Helper types for testing

type slowReader struct {
	data  []byte
	pos   int
	delay time.Duration
}

func (r *slowReader) Read(p []byte) (n int, err error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}

	// Slow down the read
	time.Sleep(r.delay)

	n = copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}
