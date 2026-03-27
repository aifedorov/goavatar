package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/aifedorov/goavatar/internal/domain"
	"github.com/aifedorov/goavatar/internal/handlers/mocks"
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
						ProcessingStatus: domain.StatusPending,
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)

			mockUploader := mocks.NewMockAvatarUploader(ctrl)
			if tt.setupMock != nil {
				tt.setupMock(mockUploader)
			}

			handler := NewAvatarHandler(mockUploader, zap.NewNop())

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
