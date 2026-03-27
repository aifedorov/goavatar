package services

import (
	"context"
	"fmt"
	"io"

	"github.com/aifedorov/goavatar/internal/domain"
	"github.com/google/uuid"
)

type S3KeyFunc func(avatarID uuid.UUID, userID, fileName string) string

type AvatarService struct {
	repo      domain.AvatarRepository
	storage   domain.FileStorage
	s3KeyFunc S3KeyFunc
}

func NewAvatarService(repo domain.AvatarRepository, storage domain.FileStorage, s3KeyFunc S3KeyFunc) *AvatarService {
	return &AvatarService{
		repo:      repo,
		storage:   storage,
		s3KeyFunc: s3KeyFunc,
	}
}

func (s *AvatarService) Upload(ctx context.Context, userID, fileName, mimeType string, sizeBytes int64, file io.Reader) (*domain.Avatar, error) {
	if userID == "" {
		return nil, fmt.Errorf("upload avatar: %w", &domain.ValidationError{Message: "user ID is required"})
	}
	if fileName == "" {
		return nil, fmt.Errorf("upload avatar: %w", &domain.ValidationError{Message: "file name is required"})
	}
	if sizeBytes <= 0 {
		return nil, fmt.Errorf("upload avatar: %w", &domain.ValidationError{Message: "file size must be positive"})
	}
	if s.storage == nil {
		return nil, fmt.Errorf("upload avatar: file storage not configured")
	}
	if s.s3KeyFunc == nil {
		return nil, fmt.Errorf("upload avatar: S3 key function not configured")
	}

	avatarID := uuid.New()
	s3Key := s.s3KeyFunc(avatarID, userID, fileName)
	if s3Key == "" {
		return nil, fmt.Errorf("upload avatar: empty storage key generated")
	}

	if err := s.storage.Upload(ctx, s3Key, file, mimeType); err != nil {
		return nil, fmt.Errorf("upload avatar to storage: %w", err)
	}

	avatar := &domain.Avatar{
		ID:               avatarID,
		UserID:           userID,
		FileName:         fileName,
		MIMEType:         mimeType,
		SizeBytes:        sizeBytes,
		S3Key:            s3Key,
		UploadStatus:     domain.StatusUploaded,
		ProcessingStatus: domain.StatusPending,
	}

	if err := s.repo.Create(ctx, avatar); err != nil {
		return nil, fmt.Errorf("save avatar metadata: %w", err)
	}

	return avatar, nil
}

// TODO: pass a real S3KeyFunc to NewAvatarService.
// The function receives avatarID, userID, fileName and returns the S3 object key.
// Consider: flat vs hierarchical layout, partitioning by user vs by avatar,
// avoiding name collisions, and how thumbnails will be organized alongside originals later.
// Example: func(id uuid.UUID, uid, name string) string { return fmt.Sprintf("originals/%s/%s", id, name) }
