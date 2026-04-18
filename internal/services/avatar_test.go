package services

import (
	"context"
	"errors"
	"fmt"
	"io"
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

func TestAvatarService_GetByID(t *testing.T) {
	testID := uuid.MustParse("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee")

	tests := []struct {
		name      string
		setupMock func(repo *mocks.MockAvatarRepository)
		wantErr   string
	}{
		{
			name: "success",
			setupMock: func(repo *mocks.MockAvatarRepository) {
				repo.EXPECT().
					GetByID(gomock.Any(), testID).
					Return(&domain.Avatar{ID: testID, UserID: "user-123"}, nil)
			},
		},
		{
			name: "not found",
			setupMock: func(repo *mocks.MockAvatarRepository) {
				repo.EXPECT().
					GetByID(gomock.Any(), testID).
					Return(nil, domain.ErrNotFound)
			},
			wantErr: "not found",
		},
		{
			name: "repo error",
			setupMock: func(repo *mocks.MockAvatarRepository) {
				repo.EXPECT().
					GetByID(gomock.Any(), testID).
					Return(nil, fmt.Errorf("connection refused"))
			},
			wantErr: "get avatar",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mockRepo := mocks.NewMockAvatarRepository(ctrl)
			tt.setupMock(mockRepo)

			svc := NewAvatarService(mockRepo, nil, nil)
			avatar, err := svc.GetByID(context.Background(), testID)

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				assert.Nil(t, avatar)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, avatar)
			assert.Equal(t, testID, avatar.ID)
		})
	}
}

func TestAvatarService_GetLatestByUserID(t *testing.T) {
	tests := []struct {
		name              string
		userID            string
		setupMock         func(repo *mocks.MockAvatarRepository)
		wantErr           string
		wantValidationErr bool
	}{
		{
			name:   "success",
			userID: "user-123",
			setupMock: func(repo *mocks.MockAvatarRepository) {
				repo.EXPECT().
					GetLatestByUserID(gomock.Any(), "user-123").
					Return(&domain.Avatar{UserID: "user-123"}, nil)
			},
		},
		{
			name:              "empty user ID",
			userID:            "",
			wantErr:           "user ID is required",
			wantValidationErr: true,
		},
		{
			name:   "not found",
			userID: "user-999",
			setupMock: func(repo *mocks.MockAvatarRepository) {
				repo.EXPECT().
					GetLatestByUserID(gomock.Any(), "user-999").
					Return(nil, domain.ErrNotFound)
			},
			wantErr: "not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mockRepo := mocks.NewMockAvatarRepository(ctrl)
			if tt.setupMock != nil {
				tt.setupMock(mockRepo)
			}

			svc := NewAvatarService(mockRepo, nil, nil)
			avatar, err := svc.GetLatestByUserID(context.Background(), tt.userID)

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				assert.Nil(t, avatar)
				if tt.wantValidationErr {
					var validErr *domain.ValidationError
					assert.True(t, errors.As(err, &validErr))
				}
				return
			}

			require.NoError(t, err)
			require.NotNil(t, avatar)
			assert.Equal(t, tt.userID, avatar.UserID)
		})
	}
}

func TestAvatarService_GetImage(t *testing.T) {
	testID := uuid.MustParse("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee")

	tests := []struct {
		name       string
		nilStorage bool
		setupMocks func(repo *mocks.MockAvatarRepository, storage *mocks.MockFileStorage)
		wantErr    string
	}{
		{
			name: "success",
			setupMocks: func(repo *mocks.MockAvatarRepository, storage *mocks.MockFileStorage) {
				repo.EXPECT().
					GetByID(gomock.Any(), testID).
					Return(&domain.Avatar{ID: testID, S3Key: "originals/test.jpg", MIMEType: "image/jpeg"}, nil)
				storage.EXPECT().
					Download(gomock.Any(), "originals/test.jpg").
					Return(io.NopCloser(strings.NewReader("image data")), nil)
			},
		},
		{
			name: "not found",
			setupMocks: func(repo *mocks.MockAvatarRepository, _ *mocks.MockFileStorage) {
				repo.EXPECT().
					GetByID(gomock.Any(), testID).
					Return(nil, domain.ErrNotFound)
			},
			wantErr: "not found",
		},
		{
			name:       "nil storage",
			nilStorage: true,
			setupMocks: func(repo *mocks.MockAvatarRepository, _ *mocks.MockFileStorage) {
				repo.EXPECT().
					GetByID(gomock.Any(), testID).
					Return(&domain.Avatar{ID: testID, S3Key: "originals/test.jpg"}, nil)
			},
			wantErr: "file storage not configured",
		},
		{
			name: "download error",
			setupMocks: func(repo *mocks.MockAvatarRepository, storage *mocks.MockFileStorage) {
				repo.EXPECT().
					GetByID(gomock.Any(), testID).
					Return(&domain.Avatar{ID: testID, S3Key: "originals/test.jpg"}, nil)
				storage.EXPECT().
					Download(gomock.Any(), "originals/test.jpg").
					Return(nil, fmt.Errorf("s3 timeout"))
			},
			wantErr: "download avatar image",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mockRepo := mocks.NewMockAvatarRepository(ctrl)
			mockStorage := mocks.NewMockFileStorage(ctrl)

			if tt.setupMocks != nil {
				tt.setupMocks(mockRepo, mockStorage)
			}

			var svc *AvatarService
			if tt.nilStorage {
				svc = NewAvatarService(mockRepo, nil, nil)
			} else {
				svc = NewAvatarService(mockRepo, mockStorage, nil)
			}

			avatar, reader, err := svc.GetImage(context.Background(), testID, "")

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				assert.Nil(t, avatar)
				assert.Nil(t, reader)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, avatar)
			require.NotNil(t, reader)
			defer reader.Close()
		})
	}
}

func TestAvatarService_GetUserImage(t *testing.T) {
	tests := []struct {
		name              string
		userID            string
		nilStorage        bool
		setupMocks        func(repo *mocks.MockAvatarRepository, storage *mocks.MockFileStorage)
		wantErr           string
		wantValidationErr bool
	}{
		{
			name:   "success",
			userID: "user-123",
			setupMocks: func(repo *mocks.MockAvatarRepository, storage *mocks.MockFileStorage) {
				repo.EXPECT().
					GetLatestByUserID(gomock.Any(), "user-123").
					Return(&domain.Avatar{UserID: "user-123", S3Key: "originals/test.jpg"}, nil)
				storage.EXPECT().
					Download(gomock.Any(), "originals/test.jpg").
					Return(io.NopCloser(strings.NewReader("image data")), nil)
			},
		},
		{
			name:              "empty user ID",
			userID:            "",
			wantErr:           "user ID is required",
			wantValidationErr: true,
		},
		{
			name:   "not found",
			userID: "user-999",
			setupMocks: func(repo *mocks.MockAvatarRepository, _ *mocks.MockFileStorage) {
				repo.EXPECT().
					GetLatestByUserID(gomock.Any(), "user-999").
					Return(nil, domain.ErrNotFound)
			},
			wantErr: "not found",
		},
		{
			name:       "nil storage",
			userID:     "user-123",
			nilStorage: true,
			setupMocks: func(repo *mocks.MockAvatarRepository, _ *mocks.MockFileStorage) {
				repo.EXPECT().
					GetLatestByUserID(gomock.Any(), "user-123").
					Return(&domain.Avatar{UserID: "user-123", S3Key: "originals/test.jpg"}, nil)
			},
			wantErr: "file storage not configured",
		},
		{
			name:   "download error",
			userID: "user-123",
			setupMocks: func(repo *mocks.MockAvatarRepository, storage *mocks.MockFileStorage) {
				repo.EXPECT().
					GetLatestByUserID(gomock.Any(), "user-123").
					Return(&domain.Avatar{UserID: "user-123", S3Key: "originals/test.jpg"}, nil)
				storage.EXPECT().
					Download(gomock.Any(), "originals/test.jpg").
					Return(nil, fmt.Errorf("s3 timeout"))
			},
			wantErr: "download user avatar image",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mockRepo := mocks.NewMockAvatarRepository(ctrl)
			mockStorage := mocks.NewMockFileStorage(ctrl)

			if tt.setupMocks != nil {
				tt.setupMocks(mockRepo, mockStorage)
			}

			var svc *AvatarService
			if tt.nilStorage {
				svc = NewAvatarService(mockRepo, nil, nil)
			} else {
				svc = NewAvatarService(mockRepo, mockStorage, nil)
			}

			avatar, reader, err := svc.GetUserImage(context.Background(), tt.userID, "")

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				assert.Nil(t, avatar)
				assert.Nil(t, reader)
				if tt.wantValidationErr {
					var validErr *domain.ValidationError
					assert.True(t, errors.As(err, &validErr))
				}
				return
			}

			require.NoError(t, err)
			require.NotNil(t, avatar)
			require.NotNil(t, reader)
			defer reader.Close()
		})
	}
}
