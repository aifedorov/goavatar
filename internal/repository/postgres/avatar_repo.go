package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/aifedorov/goavatar/internal/domain"
	"github.com/aifedorov/goavatar/internal/repository/postgres/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type AvatarRepo struct {
	q *db.Queries
}

func NewAvatarRepo(pool *pgxpool.Pool) *AvatarRepo {
	return &AvatarRepo{q: db.New(pool)}
}

func (r *AvatarRepo) Create(ctx context.Context, avatar *domain.Avatar) error {
	row, err := r.q.CreateAvatar(ctx, db.CreateAvatarParams{
		ID:        pgtype.UUID{Bytes: avatar.ID, Valid: true},
		UserID:    avatar.UserID,
		FileName:  avatar.FileName,
		MimeType:  avatar.MIMEType,
		SizeBytes: avatar.SizeBytes,
		S3Key:     avatar.S3Key,
	})
	if err != nil {
		return fmt.Errorf("create avatar: %w", err)
	}

	avatar.UploadStatus = domain.UploadStatus(row.UploadStatus)
	avatar.ProcessingStatus = domain.ProcessingStatus(row.ProcessingStatus)
	avatar.CreatedAt = row.CreatedAt.Time
	avatar.UpdatedAt = row.UpdatedAt.Time

	return nil
}

func (r *AvatarRepo) SetUploaded(ctx context.Context, id uuid.UUID) error {
	if err := r.q.SetAvatarUploaded(ctx, pgtype.UUID{Bytes: id, Valid: true}); err != nil {
		return fmt.Errorf("set avatar uploaded: %w", err)
	}
	return nil
}

func (r *AvatarRepo) SetUploadFailed(ctx context.Context, id uuid.UUID) error {
	if err := r.q.SetAvatarUploadFailed(ctx, pgtype.UUID{Bytes: id, Valid: true}); err != nil {
		return fmt.Errorf("set avatar upload failed: %w", err)
	}
	return nil
}

func (r *AvatarRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.Avatar, error) {
	row, err := r.q.GetAvatarByID(ctx, pgtype.UUID{Bytes: id, Valid: true})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("get avatar by id: %w", err)
	}
	return toDomainAvatar(row), nil
}

func (r *AvatarRepo) GetLatestByUserID(ctx context.Context, userID string) (*domain.Avatar, error) {
	row, err := r.q.GetLatestAvatarByUserID(ctx, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("get latest avatar by user id: %w", err)
	}
	return toDomainAvatar(row), nil
}

func (r *AvatarRepo) GetByUserID(ctx context.Context, userID string) ([]*domain.Avatar, error) {
	rows, err := r.q.GetAvatarsByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get avatars by user id: %w", err)
	}

	avatars := make([]*domain.Avatar, 0, len(rows))
	for _, row := range rows {
		avatars = append(avatars, toDomainAvatar(row))
	}
	return avatars, nil
}

func (r *AvatarRepo) GetProcessingStatus(ctx context.Context, id uuid.UUID) (domain.ProcessingStatus, error) {
	status, err := r.q.GetProcessingStatus(ctx, pgtype.UUID{Bytes: id, Valid: true})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", domain.ErrNotFound
		}
		return "", fmt.Errorf("get processing status: %w", err)
	}
	return domain.ProcessingStatus(status), nil
}

func (r *AvatarRepo) UpdateProcessingStatus(ctx context.Context, id uuid.UUID, status domain.ProcessingStatus, thumbnails map[string]string) error {
	var thumbJSON []byte
	if thumbnails != nil {
		var err error
		thumbJSON, err = json.Marshal(thumbnails)
		if err != nil {
			return fmt.Errorf("marshal thumbnails: %w", err)
		}
	}

	err := r.q.UpdateProcessingStatus(ctx, db.UpdateProcessingStatusParams{
		ID:               pgtype.UUID{Bytes: id, Valid: true},
		ProcessingStatus: db.ProcessingStatus(status),
		ThumbnailS3Keys:  thumbJSON,
	})
	if err != nil {
		return fmt.Errorf("update processing status: %w", err)
	}
	return nil
}

func (r *AvatarRepo) SoftDelete(ctx context.Context, id uuid.UUID, userID string) error {
	rowsAffected, err := r.q.SoftDeleteAvatar(ctx, db.SoftDeleteAvatarParams{
		ID:     pgtype.UUID{Bytes: id, Valid: true},
		UserID: userID,
	})
	if err != nil {
		return fmt.Errorf("soft delete avatar: %w", err)
	}
	if rowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func toDomainAvatar(row db.Avatar) *domain.Avatar {
	avatar := &domain.Avatar{
		ID:               row.ID.Bytes,
		UserID:           row.UserID,
		FileName:         row.FileName,
		MIMEType:         row.MimeType,
		SizeBytes:        row.SizeBytes,
		S3Key:            row.S3Key,
		UploadStatus:     domain.UploadStatus(row.UploadStatus),
		ProcessingStatus: domain.ProcessingStatus(row.ProcessingStatus),
		CreatedAt:        row.CreatedAt.Time,
		UpdatedAt:        row.UpdatedAt.Time,
	}
	if row.DeletedAt.Valid {
		t := row.DeletedAt.Time
		avatar.DeletedAt = &t
	}
	if row.ThumbnailS3Keys != nil {
		_ = json.Unmarshal(row.ThumbnailS3Keys, &avatar.ThumbnailS3Keys)
	}
	return avatar
}
