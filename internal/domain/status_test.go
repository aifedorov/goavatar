package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUploadStatus_String(t *testing.T) {
	assert.Equal(t, "uploading", UploadStatusUploading.String())
	assert.Equal(t, "uploaded", UploadStatusUploaded.String())
	assert.Equal(t, "failed", UploadStatusFailed.String())
}

func TestProcessingStatus_String(t *testing.T) {
	assert.Equal(t, "pending", ProcessingStatusPending.String())
	assert.Equal(t, "processing", ProcessingStatusProcessing.String())
	assert.Equal(t, "completed", ProcessingStatusCompleted.String())
	assert.Equal(t, "failed", ProcessingStatusFailed.String())
}
