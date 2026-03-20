package domain

import (
	"context"
	"io"
)

type FileStorage interface {
	Upload(ctx context.Context, key string, data io.Reader, contentType string) error
	Download(ctx context.Context, key string) (io.ReadCloser, error)
	Delete(ctx context.Context, key string) error
}
