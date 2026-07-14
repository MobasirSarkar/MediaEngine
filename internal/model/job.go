package model

import (
	"time"

	"github.com/google/uuid"
)

type JobStatus string

const (
	JobCreated   JobStatus = "created"
	JobQueued    JobStatus = "queued"
	JobRunning   JobStatus = "running"
	JobCompleted JobStatus = "completed"
	JobFailed    JobStatus = "failed"
)

type TaskStatus string

const (
	TaskPending    TaskStatus = "pending"
	TaskProcessing TaskStatus = "processing"
	TaskCompleted  TaskStatus = "completed"
	TaskFailed     TaskStatus = "failed"
	TaskRetrying   TaskStatus = "retrying"
)

type TaskKind string

const (
	TaskMetadata  TaskKind = "metadata"
	TaskThumbnail TaskKind = "thumbnail"
	TaskTranscode TaskKind = "transcode"
	TaskCompress  TaskKind = "compress"
)

type Job struct {
	ID        uuid.UUID `json:"id"`
	UploadID  uuid.UUID `json:"upload_id"`
	Status    JobStatus `json:"status"`
	ErrorCode string    `json:"error_code,omitempty"`
	ErrorMsg  string    `json:"error_msg,omitempty"`
	Retries   int       `json:"retries"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Task struct {
	ID         uuid.UUID  `json:"id"`
	JobID      uuid.UUID  `json:"job_id"`
	Kind       TaskKind   `json:"kind"`
	Status     TaskStatus `json:"status"`
	Attempt    int        `json:"attempt"`
	Payload    []byte     `json:"payload,omitempty"`
	Result     []byte     `json:"result,omitempty"`
	ErrorMsg   string     `json:"error_msg,omitempty"`
	StartedAt  *time.Time `json:"started_at,omitempty"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
}
