package services

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/aifedorov/goavatar/internal/domain"
	"github.com/aifedorov/goavatar/internal/domain/mocks"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestAvatarService_Upload(t *testing.T) {
	tests := []struct {
		name              string
		userID            string
		fileName          string
		sizeBytes         int64
		nilStorage        bool
		nilKeyFunc        bool
		setupMocks        func(repo *mocks.MockAvatarRepository, storage *mocks.MockFileStorage)
		wantErr           string
		wantValidationErr bool
	}{
		{
			name:      "success",
			userID:    "user-123",
			fileName:  "photo.jpg",
			sizeBytes: 1024,
			setupMocks: func(repo *mocks.MockAvatarRepository, storage *mocks.MockFileStorage) {
				storage.EXPECT().
					Upload(gomock.Any(), gomock.Any(), gomock.Any(), "image/jpeg").
					Return(nil)
				repo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					Return(nil)
			},
		},
		{
			name:              "empty user ID",
			userID:            "",
			fileName:          "photo.jpg",
			sizeBytes:         1024,
			wantErr:           "user ID is required",
			wantValidationErr: true,
		},
		{
			name:              "empty file name",
			userID:            "user-123",
			fileName:          "",
			sizeBytes:         1024,
			wantErr:           "file name is required",
			wantValidationErr: true,
		},
		{
			name:              "zero file size",
			userID:            "user-123",
			fileName:          "photo.jpg",
			sizeBytes:         0,
			wantErr:           "file size must be positive",
			wantValidationErr: true,
		},
		{
			name:              "negative file size",
			userID:            "user-123",
			fileName:          "photo.jpg",
			sizeBytes:         -1,
			wantErr:           "file size must be positive",
			wantValidationErr: true,
		},
		{
			name:       "nil storage",
			userID:     "user-123",
			fileName:   "photo.jpg",
			sizeBytes:  1024,
			nilStorage: true,
			wantErr:    "file storage not configured",
		},
		{
			name:       "empty S3 key generated",
			userID:     "user-123",
			fileName:   "photo.jpg",
			sizeBytes:  1024,
			nilKeyFunc: true,
			wantErr:    "empty storage key generated",
		},
		{
			name:      "storage upload fails",
			userID:    "user-123",
			fileName:  "photo.jpg",
			sizeBytes: 1024,
			setupMocks: func(repo *mocks.MockAvatarRepository, storage *mocks.MockFileStorage) {
				storage.EXPECT().
					Upload(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(fmt.Errorf("s3 connection refused"))
			},
			wantErr: "upload avatar to storage",
		},
		{
			name:      "repo create fails",
			userID:    "user-123",
			fileName:  "photo.jpg",
			sizeBytes: 1024,
			setupMocks: func(repo *mocks.MockAvatarRepository, storage *mocks.MockFileStorage) {
				storage.EXPECT().
					Upload(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil)
				repo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					Return(fmt.Errorf("unique constraint violation"))
			},
			wantErr: "save avatar metadata",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)

			keyFunc := S3KeyFunc(func(id uuid.UUID, userID, fileName string) string {
				return fmt.Sprintf("test/%s/%s", id, fileName)
			})
			if tt.nilKeyFunc {
				keyFunc = func(uuid.UUID, string, string) string { return "" }
			}

			mockRepo := mocks.NewMockAvatarRepository(ctrl)
			var svc *AvatarService

			if tt.nilStorage {
				svc = NewAvatarService(mockRepo, nil, keyFunc)
			} else {
				mockStorage := mocks.NewMockFileStorage(ctrl)
				if tt.setupMocks != nil {
					tt.setupMocks(mockRepo, mockStorage)
				}
				svc = NewAvatarService(mockRepo, mockStorage, keyFunc)
			}

			avatar, err := svc.Upload(
				context.Background(),
				tt.userID,
				tt.fileName,
				"image/jpeg",
				tt.sizeBytes,
				strings.NewReader("fake image data"),
			)

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				assert.Nil(t, avatar)
				if tt.wantValidationErr {
					var validErr *domain.ValidationError
					assert.True(t, errors.As(err, &validErr), "expected ValidationError, got %T", err)
				}
				return
			}

			require.NoError(t, err)
			require.NotNil(t, avatar)
			assert.NotEqual(t, uuid.Nil, avatar.ID)
			assert.Equal(t, tt.userID, avatar.UserID)
			assert.Equal(t, tt.fileName, avatar.FileName)
			assert.Equal(t, "image/jpeg", avatar.MIMEType)
			assert.Equal(t, tt.sizeBytes, avatar.SizeBytes)
			assert.Equal(t, domain.StatusUploaded, avatar.UploadStatus)
			assert.Equal(t, domain.StatusPending, avatar.ProcessingStatus)
		})
	}
}
