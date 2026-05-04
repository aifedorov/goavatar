package domain

//go:generate mockgen -destination=mocks/mock_repository.go -package=mocks . AvatarRepository

import (
	"context"

	"github.com/google/uuid"
)

type AvatarRepository interface {
	Create(ctx context.Context, avatar *Avatar) error
	GetByID(ctx context.Context, id uuid.UUID) (*Avatar, error)
	GetByUserID(ctx context.Context, userID string) ([]*Avatar, error)
	GetLatestByUserID(ctx context.Context, userID string) (*Avatar, error)
	SetUploaded(ctx context.Context, id uuid.UUID) error
	SetUploadFailed(ctx context.Context, id uuid.UUID) error
	GetProcessingStatus(ctx context.Context, id uuid.UUID) (ProcessingStatus, error)
	UpdateProcessingStatus(ctx context.Context, id uuid.UUID, status ProcessingStatus, thumbnails map[string]string) error
	SoftDelete(ctx context.Context, id uuid.UUID, userID string) error
}
