package services

import (
	"context"
	"fmt"
	"io"
	"sort"

	"github.com/aifedorov/goavatar/internal/domain"
	"github.com/google/uuid"
)

type S3KeyFunc func(avatarID uuid.UUID, userID, fileName string) string

type AvatarService struct {
	repo      domain.AvatarRepository
	storage   domain.FileStorage
	s3KeyFunc S3KeyFunc
	publisher domain.AvatarEventPublisher
}

func NewAvatarService(repo domain.AvatarRepository, storage domain.FileStorage, s3KeyFunc S3KeyFunc, publisher domain.AvatarEventPublisher) *AvatarService {
	return &AvatarService{
		repo:      repo,
		storage:   storage,
		s3KeyFunc: s3KeyFunc,
		publisher: publisher,
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

	avatar := &domain.Avatar{
		ID:        avatarID,
		UserID:    userID,
		FileName:  fileName,
		MIMEType:  mimeType,
		SizeBytes: sizeBytes,
		S3Key:     s3Key,
	}

	if err := s.repo.Create(ctx, avatar); err != nil {
		return nil, fmt.Errorf("save avatar metadata: %w", err)
	}

	if err := s.storage.Upload(ctx, s3Key, file, mimeType); err != nil {
		if failErr := s.repo.SetUploadFailed(ctx, avatarID); failErr != nil {
			return nil, fmt.Errorf("upload avatar to storage: %w (mark failed: %v)", err, failErr)
		}
		return nil, fmt.Errorf("upload avatar to storage: %w", err)
	}

	if err := s.repo.SetUploaded(ctx, avatarID); err != nil {
		return nil, fmt.Errorf("mark avatar uploaded: %w", err)
	}
	avatar.UploadStatus = domain.UploadStatusUploaded

	event := domain.AvatarUploadEvent{
		AvatarID: avatarID.String(),
		UserID:   userID,
		S3Key:    s3Key,
	}
	if err := s.publisher.PublishUploadEvent(ctx, event); err != nil {
		return nil, fmt.Errorf("publish upload event: %w", err)
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

func (s *AvatarService) Delete(ctx context.Context, id uuid.UUID, userID string) error {
	if userID == "" {
		return fmt.Errorf("delete avatar: %w", &domain.ValidationError{Message: "user ID is required"})
	}
	existing, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return fmt.Errorf("delete avatar: %w", err)
	}
	if existing.UserID != userID {
		return fmt.Errorf("delete avatar: %w", domain.ErrForbidden)
	}
	if err := s.repo.SoftDelete(ctx, id, userID); err != nil {
		return fmt.Errorf("delete avatar: %w", err)
	}
	if s.storage == nil {
		return fmt.Errorf("delete avatar: file storage not configured")
	}

	for _, key := range collectS3Keys(existing) {
		if err := s.storage.Delete(ctx, key); err != nil {
			return fmt.Errorf("delete avatar from storage: %w", err)
		}
	}

	return nil
}

func collectS3Keys(avatar *domain.Avatar) []string {
	if avatar == nil {
		return nil
	}

	keys := make([]string, 0, 1+len(avatar.ThumbnailS3Keys))
	if avatar.S3Key != "" {
		keys = append(keys, avatar.S3Key)
	}

	sizes := make([]string, 0, len(avatar.ThumbnailS3Keys))
	for size := range avatar.ThumbnailS3Keys {
		sizes = append(sizes, size)
	}
	sort.Strings(sizes)

	for _, size := range sizes {
		key := avatar.ThumbnailS3Keys[size]
		if key == "" {
			continue
		}
		keys = append(keys, key)
	}

	return keys
}

func resolveStorageKey(avatar *domain.Avatar, size string) string {
	if size == "" || size == "original" {
		return avatar.S3Key
	}
	if key, ok := avatar.ThumbnailS3Keys[size]; ok {
		return key
	}
	return avatar.S3Key
}
