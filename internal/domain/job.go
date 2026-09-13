package domain

import "time"

// JobStatus — статус задачи.
type JobStatus string

const (
	JobStatusPending    JobStatus = "pending"
	JobStatusProcessing JobStatus = "processing"
	JobStatusCompleted  JobStatus = "completed"
	JobStatusFailed     JobStatus = "failed"
)

// JobType — тип задачи.
type JobType string

const (
	JobTypeGenerateThumbnails JobType = "generate_thumbnails"
	JobTypeDeleteFiles        JobType = "delete_files"
)

// Job — доменная модель задачи на фоновую обработку.
type Job struct {
	ID          string     `json:"id"`
	Type        JobType    `json:"type"`
	AvatarID    string     `json:"avatar_id"`
	Status      JobStatus  `json:"status"`
	Attempts    int        `json:"attempts"`
	MaxAttempts int        `json:"max_attempts"`
	LastError   string     `json:"last_error,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}
