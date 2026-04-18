package worker

import (
	"context"
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

var testID = uuid.MustParse("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee")

func stubResizer() Resizer {
	return ResizeFunc(func(src io.Reader, w, h int) ([]byte, string, error) {
		return []byte("thumb"), "image/jpeg", nil
	})
}

func TestWorker_HandleUploadEvent(t *testing.T) {
	event := domain.AvatarUploadEvent{
		AvatarID: testID.String(),
		UserID:   "user-123",
		S3Key:    "originals/test.jpg",
	}

	tests := []struct {
		name       string
		resizer    Resizer
		setupMocks func(repo *mocks.MockAvatarRepository, storage *mocks.MockFileStorage)
		wantErr    string
	}{
		{
			name:    "happy path generates two thumbnails and updates DB",
			resizer: stubResizer(),
			setupMocks: func(repo *mocks.MockAvatarRepository, storage *mocks.MockFileStorage) {
				repo.EXPECT().GetProcessingStatus(gomock.Any(), testID).Return(domain.ProcessingStatusPending, nil)
				storage.EXPECT().Download(gomock.Any(), "originals/test.jpg").
					Return(io.NopCloser(strings.NewReader("image data")), nil)
				storage.EXPECT().Upload(gomock.Any(), "thumbnails/"+testID.String()+"/100x100.jpg", gomock.Any(), "image/jpeg").Return(nil)
				storage.EXPECT().Upload(gomock.Any(), "thumbnails/"+testID.String()+"/300x300.jpg", gomock.Any(), "image/jpeg").Return(nil)
				repo.EXPECT().UpdateProcessingStatus(gomock.Any(), testID, domain.ProcessingStatusCompleted,
					map[string]string{
						"100x100": "thumbnails/" + testID.String() + "/100x100.jpg",
						"300x300": "thumbnails/" + testID.String() + "/300x300.jpg",
					}).Return(nil)
			},
		},
		{
			name:    "already completed skips processing",
			resizer: stubResizer(),
			setupMocks: func(repo *mocks.MockAvatarRepository, storage *mocks.MockFileStorage) {
				repo.EXPECT().GetProcessingStatus(gomock.Any(), testID).Return(domain.ProcessingStatusCompleted, nil)
			},
		},
		{
			name:    "download failure returns error",
			resizer: stubResizer(),
			setupMocks: func(repo *mocks.MockAvatarRepository, storage *mocks.MockFileStorage) {
				repo.EXPECT().GetProcessingStatus(gomock.Any(), testID).Return(domain.ProcessingStatusPending, nil)
				storage.EXPECT().Download(gomock.Any(), "originals/test.jpg").
					Return(nil, fmt.Errorf("s3 timeout"))
			},
			wantErr: "download original",
		},
		{
			name: "resize failure returns error without DB update",
			resizer: ResizeFunc(func(io.Reader, int, int) ([]byte, string, error) {
				return nil, "", fmt.Errorf("corrupt image")
			}),
			setupMocks: func(repo *mocks.MockAvatarRepository, storage *mocks.MockFileStorage) {
				repo.EXPECT().GetProcessingStatus(gomock.Any(), testID).Return(domain.ProcessingStatusPending, nil)
				storage.EXPECT().Download(gomock.Any(), "originals/test.jpg").
					Return(io.NopCloser(strings.NewReader("image data")), nil)
			},
			wantErr: "resize 100x100",
		},
		{
			name:    "thumbnail upload failure returns error",
			resizer: stubResizer(),
			setupMocks: func(repo *mocks.MockAvatarRepository, storage *mocks.MockFileStorage) {
				repo.EXPECT().GetProcessingStatus(gomock.Any(), testID).Return(domain.ProcessingStatusPending, nil)
				storage.EXPECT().Download(gomock.Any(), "originals/test.jpg").
					Return(io.NopCloser(strings.NewReader("image data")), nil)
				storage.EXPECT().Upload(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(fmt.Errorf("s3 write error"))
			},
			wantErr: "upload thumbnail",
		},
		{
			name:    "DB update failure returns error",
			resizer: stubResizer(),
			setupMocks: func(repo *mocks.MockAvatarRepository, storage *mocks.MockFileStorage) {
				repo.EXPECT().GetProcessingStatus(gomock.Any(), testID).Return(domain.ProcessingStatusPending, nil)
				storage.EXPECT().Download(gomock.Any(), "originals/test.jpg").
					Return(io.NopCloser(strings.NewReader("image data")), nil)
				storage.EXPECT().Upload(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(2)
				repo.EXPECT().UpdateProcessingStatus(gomock.Any(), testID, gomock.Any(), gomock.Any()).
					Return(fmt.Errorf("db down"))
			},
			wantErr: "update processing status",
		},
		{
			name:    "get processing status failure returns error",
			resizer: stubResizer(),
			setupMocks: func(repo *mocks.MockAvatarRepository, storage *mocks.MockFileStorage) {
				repo.EXPECT().GetProcessingStatus(gomock.Any(), testID).Return(domain.ProcessingStatus(""), fmt.Errorf("db unreachable"))
			},
			wantErr: "get processing status",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mockRepo := mocks.NewMockAvatarRepository(ctrl)
			mockStorage := mocks.NewMockFileStorage(ctrl)
			tt.setupMocks(mockRepo, mockStorage)

			w := NewWorker(mockRepo, mockStorage, tt.resizer)
			err := w.HandleUploadEvent(context.Background(), event)

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestWorker_HandleDeleteEvent(t *testing.T) {
	t.Run("deletes all keys", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockStorage := mocks.NewMockFileStorage(ctrl)
		mockStorage.EXPECT().Delete(gomock.Any(), "originals/a.jpg").Return(nil)
		mockStorage.EXPECT().Delete(gomock.Any(), "thumbs/a-100.jpg").Return(nil)

		w := NewWorker(nil, mockStorage, nil)
		err := w.HandleDeleteEvent(context.Background(), domain.AvatarDeleteEvent{
			AvatarID: testID.String(),
			S3Keys:   []string{"originals/a.jpg", "thumbs/a-100.jpg"},
		})
		require.NoError(t, err)
	})

	t.Run("tolerates already-deleted keys", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockStorage := mocks.NewMockFileStorage(ctrl)
		mockStorage.EXPECT().Delete(gomock.Any(), "originals/a.jpg").Return(fmt.Errorf("not found"))

		w := NewWorker(nil, mockStorage, nil)
		err := w.HandleDeleteEvent(context.Background(), domain.AvatarDeleteEvent{
			AvatarID: testID.String(),
			S3Keys:   []string{"originals/a.jpg"},
		})
		require.NoError(t, err)
	})
}

func TestThumbKey(t *testing.T) {
	id := "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	assert.Equal(t, "thumbnails/"+id+"/100x100.jpg", ThumbKey(id, "100x100"))
	assert.Equal(t, "thumbnails/"+id+"/300x300.jpg", ThumbKey(id, "300x300"))
}
