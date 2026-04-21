package worker

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"github.com/aifedorov/goavatar/internal/domain"
	"github.com/google/uuid"
)

type Resizer interface {
	Resize(src io.Reader, w, h int) ([]byte, string, error)
}

type ResizeFunc func(src io.Reader, w, h int) ([]byte, string, error)

func (f ResizeFunc) Resize(src io.Reader, w, h int) ([]byte, string, error) {
	return f(src, w, h)
}

var thumbnailSizes = []struct {
	name string
	w, h int
}{
	{"100x100", 100, 100},
	{"300x300", 300, 300},
}

type Worker struct {
	repo    domain.AvatarRepository
	storage domain.FileStorage
	resizer Resizer
}

func NewWorker(repo domain.AvatarRepository, storage domain.FileStorage, resizer Resizer) *Worker {
	return &Worker{
		repo:    repo,
		storage: storage,
		resizer: resizer,
	}
}

func ThumbKey(avatarID, size string) string {
	return fmt.Sprintf("thumbnails/%s/%s.jpg", avatarID, size)
}

func (w *Worker) HandleUploadEvent(ctx context.Context, event domain.AvatarUploadEvent) error {
	avatarID, err := uuid.Parse(event.AvatarID)
	if err != nil {
		return fmt.Errorf("parse avatar ID: %w", err)
	}

	status, err := w.repo.GetProcessingStatus(ctx, avatarID)
	if err != nil {
		return fmt.Errorf("get processing status: %w", err)
	}
	if status == domain.ProcessingStatusCompleted {
		return nil
	}

	reader, err := w.storage.Download(ctx, event.S3Key)
	if err != nil {
		return fmt.Errorf("download original: %w", err)
	}
	defer reader.Close()

	original, err := io.ReadAll(reader)
	if err != nil {
		return fmt.Errorf("read original: %w", err)
	}

	thumbs := make(map[string]string, len(thumbnailSizes))
	for _, size := range thumbnailSizes {
		resized, mime, err := w.resizer.Resize(bytes.NewReader(original), size.w, size.h)
		if err != nil {
			return fmt.Errorf("resize %s: %w", size.name, err)
		}

		key := ThumbKey(event.AvatarID, size.name)
		if err := w.storage.Upload(ctx, key, bytes.NewReader(resized), mime); err != nil {
			return fmt.Errorf("upload thumbnail %s: %w", size.name, err)
		}
		thumbs[size.name] = key
	}

	if err := w.repo.UpdateProcessingStatus(ctx, avatarID, domain.ProcessingStatusCompleted, thumbs); err != nil {
		return fmt.Errorf("update processing status: %w", err)
	}

	return nil
}

func (w *Worker) HandleDeleteEvent(ctx context.Context, event domain.AvatarDeleteEvent) error {
	for _, key := range event.S3Keys {
		_ = w.storage.Delete(ctx, key)
	}
	return nil
}

func (w *Worker) MarkProcessingFailed(ctx context.Context, avatarID uuid.UUID) error {
	if err := w.repo.UpdateProcessingStatus(ctx, avatarID, domain.ProcessingStatusFailed, nil); err != nil {
		return fmt.Errorf("mark processing failed: %w", err)
	}
	return nil
}
