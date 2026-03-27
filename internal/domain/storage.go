package domain

//go:generate mockgen -destination=mocks/mock_storage.go -package=mocks . FileStorage

import (
	"context"
	"io"
)

type FileStorage interface {
	Upload(ctx context.Context, key string, data io.Reader, contentType string) error
	Download(ctx context.Context, key string) (io.ReadCloser, error)
	Delete(ctx context.Context, key string) error
}
