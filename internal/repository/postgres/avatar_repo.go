package postgres

import (
	"context"
	"fmt"

	"github.com/aifedorov/goavatar/internal/domain"
	"github.com/aifedorov/goavatar/internal/repository/postgres/db"
	"github.com/google/uuid"
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
		ID:               pgtype.UUID{Bytes: avatar.ID, Valid: true},
		UserID:           avatar.UserID,
		FileName:         avatar.FileName,
		MimeType:         avatar.MIMEType,
		SizeBytes:        avatar.SizeBytes,
		S3Key:            avatar.S3Key,
		UploadStatus:     string(avatar.UploadStatus),
		ProcessingStatus: string(avatar.ProcessingStatus),
	})
	if err != nil {
		return fmt.Errorf("create avatar: %w", err)
	}

	avatar.CreatedAt = row.CreatedAt.Time
	avatar.UpdatedAt = row.UpdatedAt.Time

	return nil
}

func (r *AvatarRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.Avatar, error) {
	return nil, fmt.Errorf("not implemented")
}

func (r *AvatarRepo) GetByUserID(ctx context.Context, userID string) ([]*domain.Avatar, error) {
	return nil, fmt.Errorf("not implemented")
}

func (r *AvatarRepo) UpdateProcessingStatus(ctx context.Context, id uuid.UUID, status domain.Status, thumbnails map[string]string) error {
	return fmt.Errorf("not implemented")
}

func (r *AvatarRepo) SoftDelete(ctx context.Context, id uuid.UUID, userID string) error {
	return fmt.Errorf("not implemented")
}
