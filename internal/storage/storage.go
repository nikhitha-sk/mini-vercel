// Package storage wraps an S3-compatible bucket (MinIO locally, Cloudflare
// R2 in production). Each deployment owns the immutable prefix <deployId>/.
package storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"path"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"

	"github.com/nikhitha-sk/mini-vercel/internal/config"
)

var ErrNotFound = errors.New("object not found")

type Storage struct {
	s3     *s3.Client
	bucket string
}

func New(ctx context.Context, c config.Config) (*Storage, error) {
	cfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(c.S3Region),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(c.S3AccessKey, c.S3SecretKey, "")),
		awsconfig.WithRequestChecksumCalculation(aws.RequestChecksumCalculationWhenRequired),
		awsconfig.WithResponseChecksumValidation(aws.ResponseChecksumValidationWhenRequired),
	)
	if err != nil {
		return nil, err
	}
	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(c.S3Endpoint)
		o.UsePathStyle = true
	})
	return &Storage{s3: client, bucket: c.S3Bucket}, nil
}

// EnsureBucket creates the bucket if it does not exist (local dev).
func (s *Storage) EnsureBucket(ctx context.Context) error {
	var lastErr error
	for i := 0; i < 30; i++ {
		_, err := s.s3.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: &s.bucket})
		if err == nil {
			return nil
		}
		_, err = s.s3.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: &s.bucket})
		if err == nil {
			return nil
		}
		var owned *types.BucketAlreadyOwnedByYou
		if errors.As(err, &owned) {
			return nil
		}
		lastErr = err
		time.Sleep(time.Second)
	}
	return fmt.Errorf("ensure bucket %q: %w", s.bucket, lastErr)
}

func (s *Storage) Put(ctx context.Context, key string, data []byte, contentType string) error {
	if contentType == "" {
		contentType = ContentType(key)
	}
	_, err := s.s3.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        &s.bucket,
		Key:           &key,
		Body:          bytes.NewReader(data),
		ContentLength: aws.Int64(int64(len(data))),
		ContentType:   &contentType,
	})
	return err
}

type Object struct {
	Body          io.ReadCloser
	ContentType   string
	ContentLength int64
	ETag          string
	LastModified  time.Time
}

func (s *Storage) Get(ctx context.Context, key string) (*Object, error) {
	out, err := s.s3.GetObject(ctx, &s3.GetObjectInput{Bucket: &s.bucket, Key: &key})
	if err != nil {
		if isNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	o := &Object{Body: out.Body, ContentType: aws.ToString(out.ContentType), ETag: aws.ToString(out.ETag)}
	if out.ContentLength != nil {
		o.ContentLength = *out.ContentLength
	}
	if out.LastModified != nil {
		o.LastModified = *out.LastModified
	}
	return o, nil
}

// Head returns metadata only (used for ETag / 304 handling and existence).
func (s *Storage) Head(ctx context.Context, key string) (*Object, error) {
	out, err := s.s3.HeadObject(ctx, &s3.HeadObjectInput{Bucket: &s.bucket, Key: &key})
	if err != nil {
		if isNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	o := &Object{ContentType: aws.ToString(out.ContentType), ETag: aws.ToString(out.ETag)}
	if out.ContentLength != nil {
		o.ContentLength = *out.ContentLength
	}
	return o, nil
}

func (s *Storage) Delete(ctx context.Context, key string) error {
	_, err := s.s3.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: &s.bucket, Key: &key})
	return err
}

func isNotFound(err error) bool {
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.ErrorCode() {
		case "NoSuchKey", "NotFound", "404":
			return true
		}
	}
	var nsk *types.NoSuchKey
	var nf *types.NotFound
	return errors.As(err, &nsk) || errors.As(err, &nf)
}

var extraTypes = map[string]string{
	".js":          "text/javascript; charset=utf-8",
	".mjs":         "text/javascript; charset=utf-8",
	".css":         "text/css; charset=utf-8",
	".html":        "text/html; charset=utf-8",
	".json":        "application/json",
	".svg":         "image/svg+xml",
	".wasm":        "application/wasm",
	".webmanifest": "application/manifest+json",
	".txt":         "text/plain; charset=utf-8",
	".map":         "application/json",
	".woff2":       "font/woff2",
	".woff":        "font/woff",
	".ico":         "image/x-icon",
	".avif":        "image/avif",
	".webp":        "image/webp",
	".xml":         "application/xml",
}

func ContentType(name string) string {
	ext := strings.ToLower(path.Ext(name))
	if t, ok := extraTypes[ext]; ok {
		return t
	}
	if t := mime.TypeByExtension(ext); t != "" {
		return t
	}
	return "application/octet-stream"
}
