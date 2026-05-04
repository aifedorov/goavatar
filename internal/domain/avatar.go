package domain

import (
	"time"

	"github.com/google/uuid"
)

type Avatar struct {
	ID               uuid.UUID
	UserID           string
	FileName         string
	MIMEType         string
	SizeBytes        int64
	S3Key            string
	ThumbnailS3Keys  map[string]string
	UploadStatus     UploadStatus
	ProcessingStatus ProcessingStatus
	CreatedAt        time.Time
	UpdatedAt        time.Time
	DeletedAt        *time.Time
}
