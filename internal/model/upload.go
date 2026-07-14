package model

import (
	"time"

	"github.com/google/uuid"
)

type UploadStatus string

const (
	UploadCreated   UploadStatus = "created"
	UploadUploading UploadStatus = "uploading"
	UploadCompleted UploadStatus = "completed"
	UploadFailed    UploadStatus = "failed"
	UploadCanceled  UploadStatus = "canceled"
	UploadExpired   UploadStatus = "expired"
)

type Upload struct {
	ID             uuid.UUID    `json:"id"`
	OwnerID        string       `json:"owner_id"`
	Filename       string       `json:"filename"`
	ContentType    string       `json:"content_type"`
	TotalSize      int64        `json:"total_size"`
	ChunkSize      int          `json:"chunk_size"`
	TotalChunks    int          `json:"total_chunks"`
	ReceivedBytes  int64        `json:"received_bytes"`
	Status         UploadStatus `json:"status"`
	Bucket         string       `json:"bucket"`
	Key            string       `json:"key"`
	ChecksumSHA256 string       `json:"checksum_sha256,omitempty"`
	CreatedAt      time.Time    `json:"created_at"`
	UpdatedAt      time.Time    `json:"updated_at"`
	ExpiresAt      time.Time    `json:"expires_at"`
}

type Chunk struct {
	UploadID   uuid.UUID `json:"upload_id"`
	ChunkNo    int       `json:"chunk_no"`
	Size       int       `json:"size"`
	Checksum   string    `json:"checksum"`
	ReceivedAt time.Time `json:"received_at"`
}
