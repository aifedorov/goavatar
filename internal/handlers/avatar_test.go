package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/aifedorov/goavatar/internal/domain"
	"github.com/aifedorov/goavatar/internal/handlers/mocks"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"go.uber.org/zap"
)

func TestAvatarHandler_Upload(t *testing.T) {
	tests := []struct {
		name       string
		userID     string
		withFile   bool
		setupMock  func(m *mocks.MockAvatarUploader)
		wantStatus int
		wantError  string
	}{
		{
			name:     "success",
			userID:   "user-123",
			withFile: true,
			setupMock: func(m *mocks.MockAvatarUploader) {
				m.EXPECT().
					Upload(gomock.Any(), "user-123", "test.jpg", gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&domain.Avatar{
						ID:               uuid.MustParse("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"),
						UserID:           "user-123",
						ProcessingStatus: domain.ProcessingStatusPending,
						CreatedAt:        time.Date(2026, 3, 27, 10, 0, 0, 0, time.UTC),
					}, nil)
			},
			wantStatus: http.StatusCreated,
		},
		{
			name:       "missing X-User-ID header",
			userID:     "",
			withFile:   false,
			wantStatus: http.StatusBadRequest,
			wantError:  "X-User-ID header is required",
		},
		{
			name:       "missing file",
			userID:     "user-123",
			withFile:   false,
			wantStatus: http.StatusBadRequest,
			wantError:  "file is required",
		},
		{
			name:     "service returns validation error",
			userID:   "user-123",
			withFile: true,
			setupMock: func(m *mocks.MockAvatarUploader) {
				m.EXPECT().
					Upload(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, fmt.Errorf("upload avatar: %w", &domain.ValidationError{Message: "file size must be positive"}))
			},
			wantStatus: http.StatusBadRequest,
			wantError:  "file size must be positive",
		},
		{
			name:     "service returns internal error",
			userID:   "user-123",
			withFile: true,
			setupMock: func(m *mocks.MockAvatarUploader) {
				m.EXPECT().
					Upload(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, fmt.Errorf("storage unavailable"))
			},
			wantStatus: http.StatusInternalServerError,
			wantError:  "internal server error",
		},
		{
			name:     "service returns file too large",
			userID:   "user-123",
			withFile: true,
			setupMock: func(m *mocks.MockAvatarUploader) {
				m.EXPECT().
					Upload(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, fmt.Errorf("upload avatar: %w", domain.ErrFileTooLarge))
			},
			wantStatus: http.StatusRequestEntityTooLarge,
			wantError:  "File too large",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)

			mockUploader := mocks.NewMockAvatarUploader(ctrl)
			if tt.setupMock != nil {
				tt.setupMock(mockUploader)
			}

			handler := NewAvatarHandler(mockUploader, nil, nil, nil, zap.NewNop(), 10*1024*1024)

			req := newMultipartRequest(t, tt.userID, tt.withFile)
			rec := httptest.NewRecorder()

			handler.Upload(rec, req)

			assert.Equal(t, tt.wantStatus, rec.Code)
			assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))

			if tt.wantError != "" {
				var resp errorResponse
				require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
				assert.Equal(t, tt.wantError, resp.Error)
			}

			if tt.wantStatus == http.StatusCreated {
				var resp uploadResponse
				require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
				assert.Equal(t, uuid.MustParse("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"), resp.ID)
				assert.Equal(t, "user-123", resp.UserID)
				assert.Equal(t, "/api/v1/avatars/aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee", resp.URL)
				assert.Equal(t, "pending", resp.Status)
				assert.Equal(t, time.Date(2026, 3, 27, 10, 0, 0, 0, time.UTC), resp.CreatedAt)
			}
		})
	}
}

func TestAvatarHandler_GetImage(t *testing.T) {
	testID := uuid.MustParse("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee")

	tests := []struct {
		name       string
		avatarID   string
		size       string
		setupMock  func(m *mocks.MockAvatarGetter)
		wantStatus int
		wantBody   string
		wantCT     string
		wantError  string
	}{
		{
			name:     "success",
			avatarID: testID.String(),
			setupMock: func(m *mocks.MockAvatarGetter) {
				m.EXPECT().
					GetImage(gomock.Any(), testID, "").
					Return(&domain.Avatar{MIMEType: "image/jpeg"}, io.NopCloser(strings.NewReader("image data")), nil)
			},
			wantStatus: http.StatusOK,
			wantBody:   "image data",
			wantCT:     "image/jpeg",
		},
		{
			name:     "success with size param",
			avatarID: testID.String(),
			size:     "100x100",
			setupMock: func(m *mocks.MockAvatarGetter) {
				m.EXPECT().
					GetImage(gomock.Any(), testID, "100x100").
					Return(&domain.Avatar{MIMEType: "image/png"}, io.NopCloser(strings.NewReader("thumb")), nil)
			},
			wantStatus: http.StatusOK,
			wantBody:   "thumb",
			wantCT:     "image/png",
		},
		{
			name:       "invalid avatar ID",
			avatarID:   "not-a-uuid",
			wantStatus: http.StatusBadRequest,
			wantError:  "invalid avatar ID",
		},
		{
			name:     "not found",
			avatarID: testID.String(),
			setupMock: func(m *mocks.MockAvatarGetter) {
				m.EXPECT().
					GetImage(gomock.Any(), testID, "").
					Return(nil, nil, fmt.Errorf("get avatar image: %w", domain.ErrNotFound))
			},
			wantStatus: http.StatusNotFound,
			wantError:  "avatar not found",
		},
		{
			name:     "internal error",
			avatarID: testID.String(),
			setupMock: func(m *mocks.MockAvatarGetter) {
				m.EXPECT().
					GetImage(gomock.Any(), testID, "").
					Return(nil, nil, fmt.Errorf("storage unavailable"))
			},
			wantStatus: http.StatusInternalServerError,
			wantError:  "internal server error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mockGetter := mocks.NewMockAvatarGetter(ctrl)
			if tt.setupMock != nil {
				tt.setupMock(mockGetter)
			}

			handler := NewAvatarHandler(nil, mockGetter, nil, nil, zap.NewNop(), 10*1024*1024)

			target := "/api/v1/avatars/" + tt.avatarID
			if tt.size != "" {
				target += "?size=" + tt.size
			}
			req := httptest.NewRequest(http.MethodGet, target, nil)
			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("avatar_id", tt.avatarID)
			req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

			rec := httptest.NewRecorder()
			handler.GetImage(rec, req)

			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantError != "" {
				var resp errorResponse
				require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
				assert.Equal(t, tt.wantError, resp.Error)
			}
			if tt.wantBody != "" {
				assert.Equal(t, tt.wantBody, rec.Body.String())
				assert.Equal(t, tt.wantCT, rec.Header().Get("Content-Type"))
			}
		})
	}
}

func TestAvatarHandler_GetUserAvatar(t *testing.T) {
	tests := []struct {
		name       string
		userID     string
		setupMock  func(m *mocks.MockAvatarGetter)
		wantStatus int
		wantBody   string
		wantCT     string
		wantError  string
	}{
		{
			name:   "success",
			userID: "user-123",
			setupMock: func(m *mocks.MockAvatarGetter) {
				m.EXPECT().
					GetUserImage(gomock.Any(), "user-123", "").
					Return(&domain.Avatar{MIMEType: "image/png"}, io.NopCloser(strings.NewReader("avatar data")), nil)
			},
			wantStatus: http.StatusOK,
			wantBody:   "avatar data",
			wantCT:     "image/png",
		},
		{
			name:   "not found",
			userID: "user-999",
			setupMock: func(m *mocks.MockAvatarGetter) {
				m.EXPECT().
					GetUserImage(gomock.Any(), "user-999", "").
					Return(nil, nil, fmt.Errorf("get user avatar image: %w", domain.ErrNotFound))
			},
			wantStatus: http.StatusNotFound,
			wantError:  "avatar not found",
		},
		{
			name:   "internal error",
			userID: "user-123",
			setupMock: func(m *mocks.MockAvatarGetter) {
				m.EXPECT().
					GetUserImage(gomock.Any(), "user-123", "").
					Return(nil, nil, fmt.Errorf("storage broke"))
			},
			wantStatus: http.StatusInternalServerError,
			wantError:  "internal server error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mockGetter := mocks.NewMockAvatarGetter(ctrl)
			if tt.setupMock != nil {
				tt.setupMock(mockGetter)
			}

			handler := NewAvatarHandler(nil, mockGetter, nil, nil, zap.NewNop(), 10*1024*1024)

			req := httptest.NewRequest(http.MethodGet, "/api/v1/users/"+tt.userID+"/avatar", nil)
			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("user_id", tt.userID)
			req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

			rec := httptest.NewRecorder()
			handler.GetUserAvatar(rec, req)

			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantError != "" {
				var resp errorResponse
				require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
				assert.Equal(t, tt.wantError, resp.Error)
			}
			if tt.wantBody != "" {
				assert.Equal(t, tt.wantBody, rec.Body.String())
				assert.Equal(t, tt.wantCT, rec.Header().Get("Content-Type"))
			}
		})
	}
}

func TestAvatarHandler_GetMetadata(t *testing.T) {
	testID := uuid.MustParse("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee")
	testTime := time.Date(2026, 3, 27, 10, 0, 0, 0, time.UTC)

	tests := []struct {
		name       string
		avatarID   string
		setupMock  func(m *mocks.MockAvatarGetter)
		wantStatus int
		wantError  string
		checkBody  func(t *testing.T, body []byte)
	}{
		{
			name:     "success without thumbnails",
			avatarID: testID.String(),
			setupMock: func(m *mocks.MockAvatarGetter) {
				m.EXPECT().
					GetByID(gomock.Any(), testID).
					Return(&domain.Avatar{
						ID:        testID,
						UserID:    "user-123",
						FileName:  "photo.jpg",
						MIMEType:  "image/jpeg",
						SizeBytes: 1024,
						CreatedAt: testTime,
						UpdatedAt: testTime,
					}, nil)
			},
			wantStatus: http.StatusOK,
			checkBody: func(t *testing.T, body []byte) {
				var resp metadataResponse
				require.NoError(t, json.Unmarshal(body, &resp))
				assert.Equal(t, testID, resp.ID)
				assert.Equal(t, "user-123", resp.UserID)
				assert.Equal(t, "photo.jpg", resp.FileName)
				assert.Equal(t, "image/jpeg", resp.MIMEType)
				assert.Equal(t, int64(1024), resp.Size)
				assert.Empty(t, resp.Thumbnails)
			},
		},
		{
			name:     "success with thumbnails",
			avatarID: testID.String(),
			setupMock: func(m *mocks.MockAvatarGetter) {
				m.EXPECT().
					GetByID(gomock.Any(), testID).
					Return(&domain.Avatar{
						ID:              testID,
						UserID:          "user-123",
						FileName:        "photo.jpg",
						MIMEType:        "image/jpeg",
						SizeBytes:       2048,
						ThumbnailS3Keys: map[string]string{"100x100": "thumb/100.jpg", "300x300": "thumb/300.jpg"},
						CreatedAt:       testTime,
						UpdatedAt:       testTime,
					}, nil)
			},
			wantStatus: http.StatusOK,
			checkBody: func(t *testing.T, body []byte) {
				var resp metadataResponse
				require.NoError(t, json.Unmarshal(body, &resp))
				assert.Len(t, resp.Thumbnails, 2)
				for _, thumb := range resp.Thumbnails {
					assert.Contains(t, thumb.URL, "/api/v1/avatars/")
					assert.Contains(t, thumb.URL, "?size=")
				}
			},
		},
		{
			name:       "invalid avatar ID",
			avatarID:   "bad-id",
			wantStatus: http.StatusBadRequest,
			wantError:  "invalid avatar ID",
		},
		{
			name:     "not found",
			avatarID: testID.String(),
			setupMock: func(m *mocks.MockAvatarGetter) {
				m.EXPECT().
					GetByID(gomock.Any(), testID).
					Return(nil, fmt.Errorf("get avatar: %w", domain.ErrNotFound))
			},
			wantStatus: http.StatusNotFound,
			wantError:  "avatar not found",
		},
		{
			name:     "internal error",
			avatarID: testID.String(),
			setupMock: func(m *mocks.MockAvatarGetter) {
				m.EXPECT().
					GetByID(gomock.Any(), testID).
					Return(nil, fmt.Errorf("db connection lost"))
			},
			wantStatus: http.StatusInternalServerError,
			wantError:  "internal server error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mockGetter := mocks.NewMockAvatarGetter(ctrl)
			if tt.setupMock != nil {
				tt.setupMock(mockGetter)
			}

			handler := NewAvatarHandler(nil, mockGetter, nil, nil, zap.NewNop(), 10*1024*1024)

			req := httptest.NewRequest(http.MethodGet, "/api/v1/avatars/"+tt.avatarID+"/metadata", nil)
			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("avatar_id", tt.avatarID)
			req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

			rec := httptest.NewRecorder()
			handler.GetMetadata(rec, req)

			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantError != "" {
				assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
				var resp errorResponse
				require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
				assert.Equal(t, tt.wantError, resp.Error)
			}
			if tt.checkBody != nil {
				assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
				tt.checkBody(t, rec.Body.Bytes())
			}
		})
	}
}

func TestAvatarHandler_Upload_MaxBytesReader(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockUploader := mocks.NewMockAvatarUploader(ctrl)

	// maxUploadBytes=1024, body limit = 1024 + 1MB; send >1MB to trigger
	handler := NewAvatarHandler(mockUploader, nil, nil, nil, zap.NewNop(), 1024)

	req := newMultipartRequestWithSize(t, "user-123", 2*1024*1024)
	rec := httptest.NewRecorder()

	handler.Upload(rec, req)

	assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))

	var resp map[string]any
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Equal(t, "File too large", resp["error"])
	assert.NotNil(t, resp["max_size"])
}

func TestAvatarHandler_DeleteAvatar(t *testing.T) {
	testID := uuid.MustParse("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee")

	tests := []struct {
		name       string
		userID     string
		avatarID   string
		setupMock  func(m *mocks.MockAvatarDeleter)
		wantStatus int
		wantError  string
	}{
		{
			name:     "success",
			userID:   "user-123",
			avatarID: testID.String(),
			setupMock: func(m *mocks.MockAvatarDeleter) {
				m.EXPECT().Delete(gomock.Any(), testID, "user-123").Return(nil)
			},
			wantStatus: http.StatusNoContent,
		},
		{
			name:       "missing X-User-ID",
			userID:     "",
			avatarID:   testID.String(),
			wantStatus: http.StatusBadRequest,
			wantError:  "X-User-ID header is required",
		},
		{
			name:       "invalid avatar ID",
			userID:     "user-123",
			avatarID:   "bad",
			wantStatus: http.StatusBadRequest,
			wantError:  "invalid avatar ID",
		},
		{
			name:     "not found",
			userID:   "user-123",
			avatarID: testID.String(),
			setupMock: func(m *mocks.MockAvatarDeleter) {
				m.EXPECT().Delete(gomock.Any(), testID, "user-123").Return(fmt.Errorf("delete avatar: %w", domain.ErrNotFound))
			},
			wantStatus: http.StatusNotFound,
			wantError:  "avatar not found",
		},
		{
			name:     "forbidden",
			userID:   "user-123",
			avatarID: testID.String(),
			setupMock: func(m *mocks.MockAvatarDeleter) {
				m.EXPECT().Delete(gomock.Any(), testID, "user-123").Return(fmt.Errorf("delete avatar: %w", domain.ErrForbidden))
			},
			wantStatus: http.StatusForbidden,
			wantError:  "you can only delete your own avatars",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mockDeleter := mocks.NewMockAvatarDeleter(ctrl)
			if tt.setupMock != nil {
				tt.setupMock(mockDeleter)
			}

			handler := NewAvatarHandler(nil, nil, mockDeleter, nil, zap.NewNop(), 10*1024*1024)

			req := httptest.NewRequest(http.MethodDelete, "/api/v1/avatars/"+tt.avatarID, nil)
			if tt.userID != "" {
				req.Header.Set("X-User-ID", tt.userID)
			}
			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("avatar_id", tt.avatarID)
			req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

			rec := httptest.NewRecorder()
			handler.DeleteAvatar(rec, req)

			assert.Equal(t, tt.wantStatus, rec.Code)
			if tt.wantError != "" {
				var resp errorResponse
				require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
				assert.Equal(t, tt.wantError, resp.Error)
			}
		})
	}
}

func TestAvatarHandler_ListUserAvatars(t *testing.T) {
	testID := uuid.MustParse("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee")

	tests := []struct {
		name       string
		userID     string
		setupMock  func(m *mocks.MockAvatarLister)
		wantStatus int
		wantCount  int
		wantError  string
	}{
		{
			name:   "success with avatars",
			userID: "user-123",
			setupMock: func(m *mocks.MockAvatarLister) {
				m.EXPECT().ListByUserID(gomock.Any(), "user-123").Return([]*domain.Avatar{
					{ID: testID, UserID: "user-123", FileName: "a.jpg", ProcessingStatus: domain.ProcessingStatusCompleted},
				}, nil)
			},
			wantStatus: http.StatusOK,
			wantCount:  1,
		},
		{
			name:   "success empty list",
			userID: "user-123",
			setupMock: func(m *mocks.MockAvatarLister) {
				m.EXPECT().ListByUserID(gomock.Any(), "user-123").Return([]*domain.Avatar{}, nil)
			},
			wantStatus: http.StatusOK,
			wantCount:  0,
		},
		{
			name:   "service error",
			userID: "user-123",
			setupMock: func(m *mocks.MockAvatarLister) {
				m.EXPECT().ListByUserID(gomock.Any(), "user-123").Return(nil, fmt.Errorf("db error"))
			},
			wantStatus: http.StatusInternalServerError,
			wantError:  "internal server error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mockLister := mocks.NewMockAvatarLister(ctrl)
			if tt.setupMock != nil {
				tt.setupMock(mockLister)
			}

			handler := NewAvatarHandler(nil, nil, nil, mockLister, zap.NewNop(), 10*1024*1024)

			req := httptest.NewRequest(http.MethodGet, "/api/v1/users/"+tt.userID+"/avatars", nil)
			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("user_id", tt.userID)
			req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

			rec := httptest.NewRecorder()
			handler.ListUserAvatars(rec, req)

			assert.Equal(t, tt.wantStatus, rec.Code)
			if tt.wantError != "" {
				var resp errorResponse
				require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
				assert.Equal(t, tt.wantError, resp.Error)
			}
			if tt.wantCount >= 0 && tt.wantError == "" {
				var items []avatarListItem
				require.NoError(t, json.NewDecoder(rec.Body).Decode(&items))
				assert.Len(t, items, tt.wantCount)
			}
		})
	}
}

func newMultipartRequest(t *testing.T, userID string, withFile bool) *http.Request {
	t.Helper()

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	if withFile {
		part, err := writer.CreateFormFile("file", "test.jpg")
		require.NoError(t, err)
		_, err = part.Write([]byte("fake image data"))
		require.NoError(t, err)
	}
	require.NoError(t, writer.Close())

	req := httptest.NewRequest(http.MethodPost, "/api/v1/avatars", &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	if userID != "" {
		req.Header.Set("X-User-ID", userID)
	}

	return req
}

func newMultipartRequestWithSize(t *testing.T, userID string, fileSize int) *http.Request {
	t.Helper()

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, err := writer.CreateFormFile("file", "big.jpg")
	require.NoError(t, err)
	_, err = part.Write(make([]byte, fileSize))
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	req := httptest.NewRequest(http.MethodPost, "/api/v1/avatars", &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	if userID != "" {
		req.Header.Set("X-User-ID", userID)
	}
	return req
}
