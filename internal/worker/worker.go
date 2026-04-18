package worker

import (
	"context"

	"github.com/aifedorov/goavatar/internal/domain"
	"github.com/aifedorov/goavatar/internal/repository/postgres"
	"github.com/aifedorov/goavatar/internal/repository/s3"
)

type Worker interface {
	HandleUploadEvent(ctx context.Context, event domain.AvatarUploadEvent) error
}

type worker struct {
	repo        *postgres.AvatarRepo
	fileStorage *s3.Storage
}

func NewWorker(repo *postgres.AvatarRepo, fileStorage *s3.Storage) Worker {
	return &worker{
		repo:        repo,
		fileStorage: fileStorage,
	}
}

func (w *worker) HandleUploadEvent(ctx context.Context, event domain.AvatarUploadEvent) error {
	return nil
}
