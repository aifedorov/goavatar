package handlers

//go:generate mockgen -destination=mocks/mock_avatar_uploader.go -package=mocks . AvatarUploader

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/aifedorov/goavatar/internal/domain"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type AvatarUploader interface {
	Upload(ctx context.Context, userID, fileName, mimeType string, sizeBytes int64, file io.Reader) (*domain.Avatar, error)
}

type AvatarHandler struct {
	uploader AvatarUploader
	logger   *zap.Logger
}

func NewAvatarHandler(uploader AvatarUploader, logger *zap.Logger) *AvatarHandler {
	return &AvatarHandler{
		uploader: uploader,
		logger:   logger,
	}
}

type uploadResponse struct {
	ID        uuid.UUID `json:"id"`
	UserID    string    `json:"user_id"`
	URL       string    `json:"url"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

type errorResponse struct {
	Error   string `json:"error"`
	Details string `json:"details,omitempty"`
}

func (h *AvatarHandler) Upload(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("X-User-ID")
	if userID == "" {
		h.writeError(w, http.StatusBadRequest, "X-User-ID header is required")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		h.logger.Error("parse form file", zap.Error(err))
		h.writeError(w, http.StatusBadRequest, "file is required")
		return
	}
	defer file.Close()

	avatar, err := h.uploader.Upload(
		r.Context(),
		userID,
		header.Filename,
		header.Header.Get("Content-Type"),
		header.Size,
		file,
	)
	if err != nil {
		var validErr *domain.ValidationError
		if errors.As(err, &validErr) {
			h.writeError(w, http.StatusBadRequest, validErr.Message)
			return
		}
		h.logger.Error("upload avatar", zap.Error(err))
		h.writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	if err := writeJSON(w, http.StatusCreated, uploadResponse{
		ID:        avatar.ID,
		UserID:    avatar.UserID,
		URL:       fmt.Sprintf("/api/v1/avatars/%s", avatar.ID),
		Status:    avatar.ProcessingStatus.String(),
		CreatedAt: avatar.CreatedAt,
	}); err != nil {
		h.logger.Error("encode response", zap.Error(err))
	}
}

func (h *AvatarHandler) writeError(w http.ResponseWriter, status int, msg string) {
	if err := writeJSON(w, status, errorResponse{Error: msg}); err != nil {
		h.logger.Error("encode error response", zap.Error(err))
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	return json.NewEncoder(w).Encode(v)
}
