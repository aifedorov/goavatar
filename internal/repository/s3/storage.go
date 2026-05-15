package s3

import (
	"context"
	"fmt"
	"io"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"github.com/aifedorov/goavatar/internal/domain"
)

type Storage struct {
	client *minio.Client
	bucket string
}

func NewClient(endpoint, accessKey, secretKey string, useSSL bool) (*minio.Client, error) {
	baseTransport, err := minio.DefaultTransport(useSSL)
	if err != nil {
		return nil, fmt.Errorf("create minio transport: %w", err)
	}
	client, err := minio.New(endpoint, &minio.Options{
		Creds:     credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure:    useSSL,
		Transport: otelhttp.NewTransport(baseTransport),
	})
	if err != nil {
		return nil, fmt.Errorf("create minio client: %w", err)
	}
	return client, nil
}

func NewStorage(client *minio.Client, bucket string) *Storage {
	return &Storage{client: client, bucket: bucket}
}

func (s *Storage) Upload(ctx context.Context, key string, data io.Reader, contentType string) error {
	_, err := s.client.PutObject(ctx, s.bucket, key, data, -1, minio.PutObjectOptions{
		ContentType: contentType,
	})
	if err != nil {
		return fmt.Errorf("put object %s: %w", key, err)
	}
	return nil
}

func (s *Storage) Download(ctx context.Context, key string) (io.ReadCloser, error) {
	obj, err := s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("get object %s: %w", key, err)
	}

	if _, err := obj.Stat(); err != nil {
		_ = obj.Close()
		return nil, mapDownloadError(key, err)
	}
	return obj, nil
}

func (s *Storage) Delete(ctx context.Context, key string) error {
	if err := s.client.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{}); err != nil {
		return fmt.Errorf("remove object %s: %w", key, err)
	}
	return nil
}

func (s *Storage) Ping(ctx context.Context) error {
	exists, err := s.client.BucketExists(ctx, s.bucket)
	if err != nil {
		return fmt.Errorf("bucket exists: %w", err)
	}
	if !exists {
		return fmt.Errorf("bucket %s not found", s.bucket)
	}
	return nil
}

func mapDownloadError(key string, err error) error {
	switch minio.ToErrorResponse(err).Code {
	case "NoSuchKey":
		return domain.ErrNotFound
	default:
		return fmt.Errorf("stat object %s: %w", key, err)
	}
}
