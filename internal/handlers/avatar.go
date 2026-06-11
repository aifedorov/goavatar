package handlers

//go:generate mockgen -destination=mocks/mock_avatar_uploader.go -package=mocks . AvatarUploader
//go:generate mockgen -destination=mocks/mock_avatar_getter.go -package=mocks . AvatarGetter
//go:generate mockgen -destination=mocks/mock_avatar_deleter.go -package=mocks . AvatarDeleter
//go:generate mockgen -destination=mocks/mock_avatar_lister.go -package=mocks . AvatarLister

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/aifedorov/goavatar/internal/domain"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type AvatarUploader interface {
	Upload(ctx context.Context, userID, fileName, mimeType string, sizeBytes int64, file io.Reader) (*domain.Avatar, error)
}

type AvatarGetter interface {
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Avatar, error)
	GetImage(ctx context.Context, id uuid.UUID, size string) (*domain.Avatar, io.ReadCloser, error)
	GetUserImage(ctx context.Context, userID, size string) (*domain.Avatar, io.ReadCloser, error)
}

type AvatarDeleter interface {
	Delete(ctx context.Context, id uuid.UUID, userID string) error
}

type AvatarLister interface {
	ListByUserID(ctx context.Context, userID string) ([]*domain.Avatar, error)
}

type AvatarHandler struct {
	uploader       AvatarUploader
	getter         AvatarGetter
	deleter        AvatarDeleter
	lister         AvatarLister
	logger         *slog.Logger
	maxUploadBytes int64
}

func NewAvatarHandler(uploader AvatarUploader, getter AvatarGetter, deleter AvatarDeleter, lister AvatarLister, logger *slog.Logger, maxUploadBytes int64) *AvatarHandler {
	return &AvatarHandler{
		uploader:       uploader,
		getter:         getter,
		deleter:        deleter,
		lister:         lister,
		logger:         logger,
		maxUploadBytes: maxUploadBytes,
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

type fileTooLargeResponse struct {
	Error   string `json:"error"`
	MaxSize int64  `json:"max_size"`
}

func (h *AvatarHandler) Upload(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("X-User-ID")
	if userID == "" {
		h.writeError(w, http.StatusBadRequest, "X-User-ID header is required")
		return
	}

	// Headroom for multipart boundaries/headers; service enforces exact file size.
	r.Body = http.MaxBytesReader(w, r.Body, h.maxUploadBytes+1<<20)

	file, header, err := r.FormFile("file")
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			h.writeFileTooLarge(w)
			return
		}
		h.logger.Error("parse form file", slog.Any("error", err))
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
		if errors.Is(err, domain.ErrFileTooLarge) {
			h.writeFileTooLarge(w)
			return
		}
		if validErr, ok := errors.AsType[*domain.ValidationError](err); ok {
			h.writeError(w, http.StatusBadRequest, validErr.Message)
			return
		}
		h.logger.Error("upload avatar", slog.Any("error", err))
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
		h.logger.Error("encode response", slog.Any("error", err))
	}
}

func (h *AvatarHandler) GetImage(w http.ResponseWriter, r *http.Request) {
	avatarID, err := uuid.Parse(chi.URLParam(r, "avatar_id"))
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid avatar ID")
		return
	}

	size := r.URL.Query().Get("size")

	avatar, reader, err := h.getter.GetImage(r.Context(), avatarID, size)
	if err != nil {
		h.handleGetError(w, err)
		return
	}
	defer reader.Close()

	w.Header().Set("Content-Type", avatar.MIMEType)
	if _, err := io.Copy(w, reader); err != nil {
		h.logger.Error("write image response", slog.Any("error", err))
	}
}

func (h *AvatarHandler) GetUserAvatar(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "user_id")
	size := r.URL.Query().Get("size")

	avatar, reader, err := h.getter.GetUserImage(r.Context(), userID, size)
	if err != nil {
		h.handleGetError(w, err)
		return
	}
	defer reader.Close()

	w.Header().Set("Content-Type", avatar.MIMEType)
	if _, err := io.Copy(w, reader); err != nil {
		h.logger.Error("write image response", slog.Any("error", err))
	}
}

type thumbnailResponse struct {
	Size string `json:"size"`
	URL  string `json:"url"`
}

type metadataResponse struct {
	ID         uuid.UUID           `json:"id"`
	UserID     string              `json:"user_id"`
	FileName   string              `json:"file_name"`
	MIMEType   string              `json:"mime_type"`
	Size       int64               `json:"size"`
	Thumbnails []thumbnailResponse `json:"thumbnails"`
	CreatedAt  time.Time           `json:"created_at"`
	UpdatedAt  time.Time           `json:"updated_at"`
}

func (h *AvatarHandler) GetMetadata(w http.ResponseWriter, r *http.Request) {
	avatarID, err := uuid.Parse(chi.URLParam(r, "avatar_id"))
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid avatar ID")
		return
	}

	avatar, err := h.getter.GetByID(r.Context(), avatarID)
	if err != nil {
		h.handleGetError(w, err)
		return
	}

	thumbnails := make([]thumbnailResponse, 0, len(avatar.ThumbnailS3Keys))
	for size := range avatar.ThumbnailS3Keys {
		thumbnails = append(thumbnails, thumbnailResponse{
			Size: size,
			URL:  fmt.Sprintf("/api/v1/avatars/%s?size=%s", avatar.ID, size),
		})
	}

	if err := writeJSON(w, http.StatusOK, metadataResponse{
		ID:         avatar.ID,
		UserID:     avatar.UserID,
		FileName:   avatar.FileName,
		MIMEType:   avatar.MIMEType,
		Size:       avatar.SizeBytes,
		Thumbnails: thumbnails,
		CreatedAt:  avatar.CreatedAt,
		UpdatedAt:  avatar.UpdatedAt,
	}); err != nil {
		h.logger.Error("encode metadata response", slog.Any("error", err))
	}
}

func (h *AvatarHandler) DeleteAvatar(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("X-User-ID")
	if userID == "" {
		h.writeError(w, http.StatusBadRequest, "X-User-ID header is required")
		return
	}

	avatarID, err := uuid.Parse(chi.URLParam(r, "avatar_id"))
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid avatar ID")
		return
	}

	if err := h.deleter.Delete(r.Context(), avatarID, userID); err != nil {
		h.handleMutationError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *AvatarHandler) DeleteUserAvatar(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("X-User-ID")
	if userID == "" {
		h.writeError(w, http.StatusBadRequest, "X-User-ID header is required")
		return
	}

	targetUserID := chi.URLParam(r, "user_id")
	if userID != targetUserID {
		h.writeError(w, http.StatusForbidden, "you can only delete your own avatars")
		return
	}

	avatars, err := h.lister.ListByUserID(r.Context(), targetUserID)
	if err != nil {
		h.handleGetError(w, err)
		return
	}
	if len(avatars) == 0 {
		h.writeError(w, http.StatusNotFound, "avatar not found")
		return
	}

	if err := h.deleter.Delete(r.Context(), avatars[0].ID, userID); err != nil {
		h.handleMutationError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

type avatarListItem struct {
	ID        uuid.UUID `json:"id"`
	UserID    string    `json:"user_id"`
	FileName  string    `json:"file_name"`
	URL       string    `json:"url"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

func (h *AvatarHandler) ListUserAvatars(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "user_id")

	avatars, err := h.lister.ListByUserID(r.Context(), userID)
	if err != nil {
		h.handleGetError(w, err)
		return
	}

	items := make([]avatarListItem, 0, len(avatars))
	for _, a := range avatars {
		items = append(items, avatarListItem{
			ID:        a.ID,
			UserID:    a.UserID,
			FileName:  a.FileName,
			URL:       fmt.Sprintf("/api/v1/avatars/%s", a.ID),
			Status:    a.ProcessingStatus.String(),
			CreatedAt: a.CreatedAt,
		})
	}

	if err := writeJSON(w, http.StatusOK, items); err != nil {
		h.logger.Error("encode list response", slog.Any("error", err))
	}
}

func (h *AvatarHandler) handleMutationError(w http.ResponseWriter, err error) {
	if errors.Is(err, domain.ErrNotFound) {
		h.writeError(w, http.StatusNotFound, "avatar not found")
		return
	}
	if errors.Is(err, domain.ErrForbidden) {
		h.writeError(w, http.StatusForbidden, "you can only delete your own avatars")
		return
	}
	h.logger.Error("mutation error", slog.Any("error", err))
	h.writeError(w, http.StatusInternalServerError, "internal server error")
}

func (h *AvatarHandler) handleGetError(w http.ResponseWriter, err error) {
	if errors.Is(err, domain.ErrNotFound) {
		h.writeError(w, http.StatusNotFound, "avatar not found")
		return
	}
	if validErr, ok := errors.AsType[*domain.ValidationError](err); ok {
		h.writeError(w, http.StatusBadRequest, validErr.Message)
		return
	}
	h.logger.Error("get avatar", slog.Any("error", err))
	h.writeError(w, http.StatusInternalServerError, "internal server error")
}

func (h *AvatarHandler) writeFileTooLarge(w http.ResponseWriter) {
	if err := writeJSON(w, http.StatusRequestEntityTooLarge, fileTooLargeResponse{
		Error:   "File too large",
		MaxSize: h.maxUploadBytes,
	}); err != nil {
		h.logger.Error("encode file too large response", slog.Any("error", err))
	}
}

func (h *AvatarHandler) writeError(w http.ResponseWriter, status int, msg string) {
	if err := writeJSON(w, status, errorResponse{Error: msg}); err != nil {
		h.logger.Error("encode error response", slog.Any("error", err))
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	return json.NewEncoder(w).Encode(v)
}
