package filesystem

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// S3 is the S3 filesystem driver.
type S3 struct {
	client *s3.Client
	bucket string
	url    string
	region string
}

// NewS3 creates a new S3 filesystem instance.
func NewS3(config map[string]any) (*S3, error) {
	key, _ := config["key"].(string)
	secret, _ := config["secret"].(string)
	region, _ := config["region"].(string)
	bucket, ok := config["bucket"].(string)
	if !ok {
		return nil, fmt.Errorf("filesystem: bucket not defined for s3 driver")
	}
	url, _ := config["url"].(string)
	endpoint, _ := config["endpoint"].(string)
	usePathStyle, _ := config["use_path_style_endpoint"].(bool)

	// Load AWS config
	cfg, err := awsconfig.LoadDefaultConfig(context.TODO(),
		awsconfig.WithRegion(region),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(key, secret, "")),
	)
	if err != nil {
		return nil, err
	}

	// Create S3 client options
	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		if endpoint != "" {
			o.BaseEndpoint = aws.String(endpoint)
		}
		o.UsePathStyle = usePathStyle
	})

	return &S3{
		client: client,
		bucket: bucket,
		url:    url,
		region: region,
	}, nil
}

func (s *S3) Exists(ctx context.Context, path string) bool {
	_, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(path),
	})
	return err == nil
}

func (s *S3) Get(ctx context.Context, path string) (string, error) {
	b, err := s.GetBytes(ctx, path)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func (s *S3) GetBytes(ctx context.Context, path string) ([]byte, error) {
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(path),
	})
	if err != nil {
		return nil, err
	}
	defer out.Body.Close()

	return io.ReadAll(out.Body)
}

func (s *S3) Put(ctx context.Context, path string, contents string) error {
	return s.PutStream(ctx, path, strings.NewReader(contents))
}

func (s *S3) PutBytes(ctx context.Context, path string, contents []byte) error {
	return s.PutStream(ctx, path, bytes.NewReader(contents))
}

func (s *S3) PutStream(ctx context.Context, path string, contents io.Reader) error {
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(path),
		Body:   contents,
	})
	return err
}

func (s *S3) Delete(ctx context.Context, path string) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(path),
	})
	return err
}

func (s *S3) Copy(ctx context.Context, from, to string) error {
	_, err := s.client.CopyObject(ctx, &s3.CopyObjectInput{
		Bucket:     aws.String(s.bucket),
		CopySource: aws.String(fmt.Sprintf("%s/%s", s.bucket, from)),
		Key:        aws.String(to),
	})
	return err
}

// MovePartialError represents a partial failure during a Move operation:
// the copy from source to destination succeeded, but deleting the source failed.
type MovePartialError struct {
	From string
	To   string
	Err  error
}

func (e *MovePartialError) Error() string {
	return fmt.Sprintf("move %s -> %s: copy succeeded but delete failed: %v", e.From, e.To, e.Err)
}

func (e *MovePartialError) Unwrap() error {
	return e.Err
}

func (s *S3) Move(ctx context.Context, from, to string) error {
	if err := s.Copy(ctx, from, to); err != nil {
		return fmt.Errorf("move %s -> %s: copy failed: %w", from, to, err)
	}

	if err := s.Delete(ctx, from); err != nil {
		return &MovePartialError{
			From: from,
			To:   to,
			Err:  err,
		}
	}

	return nil
}

func (s *S3) Size(ctx context.Context, path string) (int64, error) {
	out, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(path),
	})
	if err != nil {
		return 0, err
	}
	return *out.ContentLength, nil
}

func (s *S3) LastModified(ctx context.Context, path string) (time.Time, error) {
	out, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(path),
	})
	if err != nil {
		return time.Time{}, err
	}
	return *out.LastModified, nil
}

func (s *S3) MakeDirectory(ctx context.Context, path string) error {
	// S3 is flat, but we can mimic directory creation by creating an empty object with trailing slash
	if !strings.HasSuffix(path, "/") {
		path = path + "/"
	}
	return s.Put(ctx, path, "")
}

func (s *S3) DeleteDirectory(ctx context.Context, path string) error {
	// List all objects provided with prefix
	if !strings.HasSuffix(path, "/") {
		path = path + "/"
	}

	// Loop and delete
	paginator := s3.NewListObjectsV2Paginator(s.client, &s3.ListObjectsV2Input{
		Bucket: aws.String(s.bucket),
		Prefix: aws.String(path),
	})

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return err
		}

		objects := make([]types.ObjectIdentifier, len(page.Contents))
		for i, obj := range page.Contents {
			objects[i] = types.ObjectIdentifier{Key: obj.Key}
		}

		if len(objects) > 0 {
			_, err := s.client.DeleteObjects(ctx, &s3.DeleteObjectsInput{
				Bucket: aws.String(s.bucket),
				Delete: &types.Delete{Objects: objects},
			})
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *S3) Url(path string) string {
	if s.url != "" {
		return strings.TrimRight(s.url, "/") + "/" + strings.TrimLeft(path, "/")
	}
	return fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", s.bucket, s.region, strings.TrimLeft(path, "/"))
}
