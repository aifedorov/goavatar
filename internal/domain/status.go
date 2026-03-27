package domain

type Status string

const (
	StatusPending    Status = "pending"
	StatusUploading  Status = "uploading"
	StatusUploaded   Status = "uploaded"
	StatusProcessing Status = "processing"
	StatusCompleted  Status = "completed"
	StatusFailed     Status = "failed"
	StatusDeleted    Status = "deleted"
)

func (s Status) String() string {
	return string(s)
}
