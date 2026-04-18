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

func (s *AvatarService) GetByID(ctx context.Context, id uuid.UUID) (*domain.Avatar, error) {
	avatar, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get avatar: %w", err)
	}
	return avatar, nil
}

func (s *AvatarService) GetLatestByUserID(ctx context.Context, userID string) (*domain.Avatar, error) {
	if userID == "" {
		return nil, fmt.Errorf("get user avatar: %w", &domain.ValidationError{Message: "user ID is required"})
	}
	avatar, err := s.repo.GetLatestByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get user avatar: %w", err)
	}
	return avatar, nil
}

func (s *AvatarService) GetImage(ctx context.Context, id uuid.UUID, size string) (*domain.Avatar, io.ReadCloser, error) {
	avatar, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, nil, fmt.Errorf("get avatar image: %w", err)
	}
	if s.storage == nil {
		return nil, nil, fmt.Errorf("get avatar image: file storage not configured")
	}

	key := resolveStorageKey(avatar, size)

	reader, err := s.storage.Download(ctx, key)
	if err != nil {
		return nil, nil, fmt.Errorf("download avatar image: %w", err)
	}

	return avatar, reader, nil
}

func (s *AvatarService) GetUserImage(ctx context.Context, userID, size string) (*domain.Avatar, io.ReadCloser, error) {
	if userID == "" {
		return nil, nil, fmt.Errorf("get user avatar image: %w", &domain.ValidationError{Message: "user ID is required"})
	}

	avatar, err := s.repo.GetLatestByUserID(ctx, userID)
	if err != nil {
		return nil, nil, fmt.Errorf("get user avatar image: %w", err)
	}
	if s.storage == nil {
		return nil, nil, fmt.Errorf("get user avatar image: file storage not configured")
	}

	key := resolveStorageKey(avatar, size)

	reader, err := s.storage.Download(ctx, key)
	if err != nil {
		return nil, nil, fmt.Errorf("download user avatar image: %w", err)
	}

	return avatar, reader, nil
}

func (s *AvatarService) ListByUserID(ctx context.Context, userID string) ([]*domain.Avatar, error) {
	if userID == "" {
		return nil, fmt.Errorf("list user avatars: %w", &domain.ValidationError{Message: "user ID is required"})
	}
	avatars, err := s.repo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list user avatars: %w", err)
	}
	return avatars, nil
}

// TODO(human): implement Delete — decide how to handle the ownership check.
// The repo.SoftDelete(ctx, id, userID) returns ErrNotFound when 0 rows affected,
// but that conflates "avatar doesn't exist" with "avatar belongs to another user".
// Choose your strategy and implement this ~5-line method body.
func (s *AvatarService) Delete(ctx context.Context, id uuid.UUID, userID string) error {
	return fmt.Errorf("not implemented")
}

// TODO: implement resolveStorageKey — selects which S3 object key to use
// when serving an avatar image based on the requested thumbnail size.
// Currently returns the original image key regardless of requested size.
func resolveStorageKey(avatar *domain.Avatar, size string) string {
	return avatar.S3Key
}

// TODO: pass a real S3KeyFunc to NewAvatarService.
// The function receives avatarID, userID, fileName and returns the S3 object key.
// Consider: flat vs hierarchical layout, partitioning by user vs by avatar,
// avoiding name collisions, and how thumbnails will be organized alongside originals later.
// Example: func(id uuid.UUID, uid, name string) string { return fmt.Sprintf("originals/%s/%s", id, name) }
