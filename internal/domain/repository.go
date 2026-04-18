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
	UpdateProcessingStatus(ctx context.Context, id uuid.UUID, status Status, thumbnails map[string]string) error
	SoftDelete(ctx context.Context, id uuid.UUID, userID string) error
}
