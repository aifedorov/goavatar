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
	UploadStatus     Status
	ProcessingStatus Status
	CreatedAt        time.Time
	UpdatedAt        time.Time
	DeletedAt        *time.Time
}
