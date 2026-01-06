package filesystem

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// Mock S3 client for testing
type mockS3Client struct {
	objects        map[string][]byte
	objectMetadata map[string]objectMeta
	headObjectErr  error
	getObjectErr   error
	putObjectErr   error
	deleteErr      error
	copyErr        error
}

type objectMeta struct {
	size         int64
	lastModified time.Time
}

func (m *mockS3Client) HeadObject(ctx context.Context, params *s3.HeadObjectInput, optFns ...func(*s3.Options)) (*s3.HeadObjectOutput, error) {
	if m.headObjectErr != nil {
		return nil, m.headObjectErr
	}

	key := aws.ToString(params.Key)
	if _, exists := m.objects[key]; !exists {
		return nil, &types.NoSuchKey{}
	}

	meta := m.objectMetadata[key]
	return &s3.HeadObjectOutput{
		ContentLength: aws.Int64(meta.size),
		LastModified:  aws.Time(meta.lastModified),
	}, nil
}

func (m *mockS3Client) GetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	if m.getObjectErr != nil {
		return nil, m.getObjectErr
	}

	key := aws.ToString(params.Key)
	data, exists := m.objects[key]
	if !exists {
		return nil, &types.NoSuchKey{}
	}

	return &s3.GetObjectOutput{
		Body: io.NopCloser(bytes.NewReader(data)),
	}, nil
}

func (m *mockS3Client) PutObject(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	if m.putObjectErr != nil {
		return nil, m.putObjectErr
	}

	key := aws.ToString(params.Key)
	data, err := io.ReadAll(params.Body)
	if err != nil {
		return nil, err
	}

	m.objects[key] = data
	m.objectMetadata[key] = objectMeta{
		size:         int64(len(data)),
		lastModified: time.Now(),
	}

	return &s3.PutObjectOutput{}, nil
}

func (m *mockS3Client) DeleteObject(ctx context.Context, params *s3.DeleteObjectInput, optFns ...func(*s3.Options)) (*s3.DeleteObjectOutput, error) {
	if m.deleteErr != nil {
		return nil, m.deleteErr
	}

	key := aws.ToString(params.Key)
	delete(m.objects, key)
	delete(m.objectMetadata, key)

	return &s3.DeleteObjectOutput{}, nil
}

func (m *mockS3Client) CopyObject(ctx context.Context, params *s3.CopyObjectInput, optFns ...func(*s3.Options)) (*s3.CopyObjectOutput, error) {
	if m.copyErr != nil {
		return nil, m.copyErr
	}

	// Parse source from CopySource (format: bucket/key)
	source := aws.ToString(params.CopySource)
	parts := strings.SplitN(source, "/", 2)
	if len(parts) < 2 {
		return nil, &types.NoSuchKey{}
	}
	sourceKey := parts[1]

	data, exists := m.objects[sourceKey]
	if !exists {
		return nil, &types.NoSuchKey{}
	}

	destKey := aws.ToString(params.Key)
	m.objects[destKey] = data
	m.objectMetadata[destKey] = objectMeta{
		size:         int64(len(data)),
		lastModified: time.Now(),
	}

	return &s3.CopyObjectOutput{}, nil
}

func (m *mockS3Client) DeleteObjects(ctx context.Context, params *s3.DeleteObjectsInput, optFns ...func(*s3.Options)) (*s3.DeleteObjectsOutput, error) {
	if m.deleteErr != nil {
		return nil, m.deleteErr
	}

	for _, obj := range params.Delete.Objects {
		key := aws.ToString(obj.Key)
		delete(m.objects, key)
		delete(m.objectMetadata, key)
	}

	return &s3.DeleteObjectsOutput{}, nil
}

func (m *mockS3Client) ListObjectsV2(ctx context.Context, params *s3.ListObjectsV2Input, optFns ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
	prefix := aws.ToString(params.Prefix)
	var contents []types.Object

	for key, data := range m.objects {
		if strings.HasPrefix(key, prefix) {
			meta := m.objectMetadata[key]
			contents = append(contents, types.Object{
				Key:          aws.String(key),
				Size:         aws.Int64(int64(len(data))),
				LastModified: aws.Time(meta.lastModified),
			})
		}
	}

	return &s3.ListObjectsV2Output{
		Contents: contents,
	}, nil
}

func newMockS3() *mockS3Client {
	return &mockS3Client{
		objects:        make(map[string][]byte),
		objectMetadata: make(map[string]objectMeta),
	}
}

func setupS3FS(t *testing.T) (*S3, *mockS3Client) {
	t.Helper()

	mock := newMockS3()
	fs := &S3{
		client: mock,
		bucket: "test-bucket",
		url:    "https://cdn.example.com",
		region: "us-east-1",
	}

	return fs, mock
}

func TestNewS3(t *testing.T) {
	t.Run("missing bucket", func(t *testing.T) {
		_, err := NewS3(map[string]any{
			"key":    "test-key",
			"secret": "test-secret",
			"region": "us-east-1",
		})

		if err == nil {
			t.Fatal("expected error for missing bucket")
		}
		if !strings.Contains(err.Error(), "bucket not defined") {
			t.Errorf("expected 'bucket not defined' error, got: %v", err)
		}
	})
}

func TestS3Exists(t *testing.T) {
	fs, mock := setupS3FS(t)
	ctx := context.Background()

	t.Run("non-existent object", func(t *testing.T) {
		if fs.Exists(ctx, "non-existent.txt") {
			t.Error("expected object to not exist")
		}
	})

	t.Run("existing object", func(t *testing.T) {
		mock.objects["test.txt"] = []byte("content")
		mock.objectMetadata["test.txt"] = objectMeta{size: 7, lastModified: time.Now()}

		if !fs.Exists(ctx, "test.txt") {
			t.Error("expected object to exist")
		}
	})
}

func TestS3GetAndPut(t *testing.T) {
	fs, _ := setupS3FS(t)
	ctx := context.Background()

	t.Run("put and get string", func(t *testing.T) {
		content := "Hello, S3!"

		if err := fs.Put(ctx, "test.txt", content); err != nil {
			t.Fatalf("failed to put object: %v", err)
		}

		got, err := fs.Get(ctx, "test.txt")
		if err != nil {
			t.Fatalf("failed to get object: %v", err)
		}

		if got != content {
			t.Errorf("expected '%s', got '%s'", content, got)
		}
	})

	t.Run("put and get bytes", func(t *testing.T) {
		content := []byte("Binary S3 content")

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

	t.Run("get non-existent object", func(t *testing.T) {
		_, err := fs.Get(ctx, "non-existent.txt")
		if err == nil {
			t.Fatal("expected error for non-existent object")
		}
	})
}

func TestS3PutStream(t *testing.T) {
	fs, _ := setupS3FS(t)
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		content := "Stream content for S3"
		reader := strings.NewReader(content)

		if err := fs.PutStream(ctx, "stream.txt", reader); err != nil {
			t.Fatalf("failed to put stream: %v", err)
		}

		got, err := fs.Get(ctx, "stream.txt")
		if err != nil {
			t.Fatalf("failed to get object: %v", err)
		}

		if got != content {
			t.Errorf("expected '%s', got '%s'", content, got)
		}
	})
}

func TestS3Delete(t *testing.T) {
	fs, mock := setupS3FS(t)
	ctx := context.Background()

	t.Run("delete existing object", func(t *testing.T) {
		if err := fs.Put(ctx, "delete-me.txt", "content"); err != nil {
			t.Fatalf("failed to create object: %v", err)
		}

		if err := fs.Delete(ctx, "delete-me.txt"); err != nil {
			t.Fatalf("failed to delete object: %v", err)
		}

		if fs.Exists(ctx, "delete-me.txt") {
			t.Error("object should not exist after delete")
		}
	})

	t.Run("delete non-existent object", func(t *testing.T) {
		// S3 DeleteObject doesn't error on non-existent objects
		err := fs.Delete(ctx, "non-existent.txt")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("delete with error", func(t *testing.T) {
		mock.deleteErr = &types.NoSuchBucket{}
		defer func() { mock.deleteErr = nil }()

		err := fs.Delete(ctx, "test.txt")
		if err == nil {
			t.Fatal("expected error from delete")
		}
	})
}

func TestS3Copy(t *testing.T) {
	fs, _ := setupS3FS(t)
	ctx := context.Background()

	t.Run("copy existing object", func(t *testing.T) {
		content := "Original S3 content"
		if err := fs.Put(ctx, "original.txt", content); err != nil {
			t.Fatalf("failed to create object: %v", err)
		}

		if err := fs.Copy(ctx, "original.txt", "copy.txt"); err != nil {
			t.Fatalf("failed to copy object: %v", err)
		}

		// Check original still exists
		if !fs.Exists(ctx, "original.txt") {
			t.Error("original object should still exist")
		}

		// Check copy has same content
		got, err := fs.Get(ctx, "copy.txt")
		if err != nil {
			t.Fatalf("failed to get copied object: %v", err)
		}
		if got != content {
			t.Errorf("expected '%s', got '%s'", content, got)
		}
	})

	t.Run("copy non-existent object", func(t *testing.T) {
		err := fs.Copy(ctx, "non-existent.txt", "copy.txt")
		if err == nil {
			t.Fatal("expected error when copying non-existent object")
		}
	})
}

func TestS3Move(t *testing.T) {
	fs, _ := setupS3FS(t)
	ctx := context.Background()

	t.Run("move existing object", func(t *testing.T) {
		content := "Move me in S3"
		if err := fs.Put(ctx, "source.txt", content); err != nil {
			t.Fatalf("failed to create object: %v", err)
		}

		if err := fs.Move(ctx, "source.txt", "destination.txt"); err != nil {
			t.Fatalf("failed to move object: %v", err)
		}

		// Check source no longer exists
		if fs.Exists(ctx, "source.txt") {
			t.Error("source object should not exist after move")
		}

		// Check destination has content
		got, err := fs.Get(ctx, "destination.txt")
		if err != nil {
			t.Fatalf("failed to get moved object: %v", err)
		}
		if got != content {
			t.Errorf("expected '%s', got '%s'", content, got)
		}
	})
}

func TestS3Size(t *testing.T) {
	fs, mock := setupS3FS(t)
	ctx := context.Background()

	t.Run("get size of object", func(t *testing.T) {
		content := "12345"
		if err := fs.Put(ctx, "sized.txt", content); err != nil {
			t.Fatalf("failed to create object: %v", err)
		}

		size, err := fs.Size(ctx, "sized.txt")
		if err != nil {
			t.Fatalf("failed to get size: %v", err)
		}

		if size != int64(len(content)) {
			t.Errorf("expected size %d, got %d", len(content), size)
		}
	})

	t.Run("size of non-existent object", func(t *testing.T) {
		_, err := fs.Size(ctx, "non-existent.txt")
		if err == nil {
			t.Fatal("expected error for non-existent object")
		}
	})

	t.Run("nil content length", func(t *testing.T) {
		// Store object first
		mock.objects["test.txt"] = []byte("content")
		mock.objectMetadata["test.txt"] = objectMeta{size: 7, lastModified: time.Now()}

		// Temporarily override HeadObject to return nil ContentLength
		oldHeadObjectErr := mock.headObjectErr
		mock.headObjectErr = nil

		// Create a custom mock for this test that returns nil ContentLength
		originalClient := fs.client
		fs.client = &mockS3ClientNilSize{mockS3Client: mock}
		defer func() {
			fs.client = originalClient
			mock.headObjectErr = oldHeadObjectErr
		}()

		_, err := fs.Size(ctx, "test.txt")
		if err == nil {
			t.Fatal("expected error for nil ContentLength")
		}
		if !strings.Contains(err.Error(), "not available") {
			t.Errorf("expected 'not available' error, got: %v", err)
		}
	})
}

func TestS3LastModified(t *testing.T) {
	fs, mock := setupS3FS(t)
	ctx := context.Background()

	t.Run("get last modified time", func(t *testing.T) {
		before := time.Now()

		if err := fs.Put(ctx, "timed.txt", "content"); err != nil {
			t.Fatalf("failed to create object: %v", err)
		}

		after := time.Now()

		modTime, err := fs.LastModified(ctx, "timed.txt")
		if err != nil {
			t.Fatalf("failed to get last modified: %v", err)
		}

		if modTime.Before(before) || modTime.After(after) {
			t.Errorf("modification time %v not between %v and %v", modTime, before, after)
		}
	})

	t.Run("last modified of non-existent object", func(t *testing.T) {
		_, err := fs.LastModified(ctx, "non-existent.txt")
		if err == nil {
			t.Fatal("expected error for non-existent object")
		}
	})

	t.Run("nil last modified", func(t *testing.T) {
		// Store object first
		mock.objects["test.txt"] = []byte("content")
		mock.objectMetadata["test.txt"] = objectMeta{size: 7, lastModified: time.Now()}

		// Create a custom mock for this test that returns nil LastModified
		originalClient := fs.client
		fs.client = &mockS3ClientNilTime{mockS3Client: mock}
		defer func() { fs.client = originalClient }()

		_, err := fs.LastModified(ctx, "test.txt")
		if err == nil {
			t.Fatal("expected error for nil LastModified")
		}
		if !strings.Contains(err.Error(), "not available") {
			t.Errorf("expected 'not available' error, got: %v", err)
		}
	})
}

func TestS3MakeDirectory(t *testing.T) {
	fs, _ := setupS3FS(t)
	ctx := context.Background()

	t.Run("create directory marker", func(t *testing.T) {
		if err := fs.MakeDirectory(ctx, "newdir"); err != nil {
			t.Fatalf("failed to create directory: %v", err)
		}

		// Check that directory marker exists
		if !fs.Exists(ctx, "newdir/") {
			t.Error("expected directory marker to exist")
		}
	})

	t.Run("add trailing slash if missing", func(t *testing.T) {
		if err := fs.MakeDirectory(ctx, "dir"); err != nil {
			t.Fatalf("failed to create directory: %v", err)
		}

		if !fs.Exists(ctx, "dir/") {
			t.Error("expected directory marker with trailing slash")
		}
	})
}

func TestS3DeleteDirectory(t *testing.T) {
	fs, _ := setupS3FS(t)
	ctx := context.Background()

	t.Run("delete directory with contents", func(t *testing.T) {
		// Create some objects in the directory
		if err := fs.Put(ctx, "dir/file1.txt", "content1"); err != nil {
			t.Fatalf("failed to create object: %v", err)
		}
		if err := fs.Put(ctx, "dir/file2.txt", "content2"); err != nil {
			t.Fatalf("failed to create object: %v", err)
		}
		if err := fs.Put(ctx, "dir/subdir/file3.txt", "content3"); err != nil {
			t.Fatalf("failed to create object: %v", err)
		}

		if err := fs.DeleteDirectory(ctx, "dir"); err != nil {
			t.Fatalf("failed to delete directory: %v", err)
		}

		// Check that all objects are deleted
		if fs.Exists(ctx, "dir/file1.txt") || fs.Exists(ctx, "dir/file2.txt") || fs.Exists(ctx, "dir/subdir/file3.txt") {
			t.Error("directory contents should be deleted")
		}
	})

	t.Run("adds trailing slash if missing", func(t *testing.T) {
		if err := fs.Put(ctx, "testdir/file.txt", "content"); err != nil {
			t.Fatalf("failed to create object: %v", err)
		}

		if err := fs.DeleteDirectory(ctx, "testdir"); err != nil {
			t.Fatalf("failed to delete directory: %v", err)
		}

		if fs.Exists(ctx, "testdir/file.txt") {
			t.Error("directory contents should be deleted")
		}
	})
}

func TestS3Url(t *testing.T) {
	t.Run("with custom url", func(t *testing.T) {
		fs := &S3{
			bucket: "test-bucket",
			url:    "https://cdn.example.com",
			region: "us-east-1",
		}

		url := fs.Url("path/to/file.txt")
		expected := "https://cdn.example.com/path/to/file.txt"
		if url != expected {
			t.Errorf("expected '%s', got '%s'", expected, url)
		}
	})

	t.Run("without custom url", func(t *testing.T) {
		fs := &S3{
			bucket: "my-bucket",
			url:    "",
			region: "us-west-2",
		}

		url := fs.Url("file.txt")
		expected := "https://my-bucket.s3.us-west-2.amazonaws.com/file.txt"
		if url != expected {
			t.Errorf("expected '%s', got '%s'", expected, url)
		}
	})

	t.Run("url with trailing slash", func(t *testing.T) {
		fs := &S3{
			bucket: "test-bucket",
			url:    "https://cdn.example.com/",
			region: "us-east-1",
		}

		url := fs.Url("file.txt")
		expected := "https://cdn.example.com/file.txt"
		if url != expected {
			t.Errorf("expected '%s', got '%s'", expected, url)
		}
	})

	t.Run("path with leading slash", func(t *testing.T) {
		fs := &S3{
			bucket: "test-bucket",
			url:    "https://cdn.example.com",
			region: "us-east-1",
		}

		url := fs.Url("/file.txt")
		expected := "https://cdn.example.com/file.txt"
		if url != expected {
			t.Errorf("expected '%s', got '%s'", expected, url)
		}
	})
}

// Custom mock for testing nil ContentLength
type mockS3ClientNilSize struct {
	*mockS3Client
}

func (m *mockS3ClientNilSize) HeadObject(ctx context.Context, params *s3.HeadObjectInput, optFns ...func(*s3.Options)) (*s3.HeadObjectOutput, error) {
	key := aws.ToString(params.Key)
	if _, exists := m.objects[key]; !exists {
		return nil, &types.NoSuchKey{}
	}

	meta := m.objectMetadata[key]
	return &s3.HeadObjectOutput{
		ContentLength: nil, // Intentionally nil
		LastModified:  aws.Time(meta.lastModified),
	}, nil
}

// Custom mock for testing nil LastModified
type mockS3ClientNilTime struct {
	*mockS3Client
}

func (m *mockS3ClientNilTime) HeadObject(ctx context.Context, params *s3.HeadObjectInput, optFns ...func(*s3.Options)) (*s3.HeadObjectOutput, error) {
	key := aws.ToString(params.Key)
	if _, exists := m.objects[key]; !exists {
		return nil, &types.NoSuchKey{}
	}

	meta := m.objectMetadata[key]
	return &s3.HeadObjectOutput{
		ContentLength: aws.Int64(meta.size),
		LastModified:  nil, // Intentionally nil
	}, nil
}
