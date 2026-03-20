package domain

type Status string

const (
	StatusPending    Status = "pending"
	StatusUploaded   Status = "uploaded"
	StatusProcessing Status = "processing"
	StatusFailed     Status = "failed"
	StatusDeleted    Status = "deleted"
)

func (s Status) String() string {
	return string(s)
}
