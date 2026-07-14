package upload

import (
	"time"

	"github.com/google/uuid"

	"github.com/MobasirSarkar/MediaEngine/internal/model"
)

type CreateReq struct {
	OwnerID     string `json:"owner_id" validate:"required,min=1,max=128"`
	Filename    string `json:"filename" validate:"required,min=1,max=255"`
	ContentType string `json:"content_type" validate:"required,min=1,max=127"`
	TotalSize   int64  `json:"total_size" validate:"required,gt=0"`
	ChunkSize   int64  `json:"chunk_size" validate:"required,gte=5242880,lte=104857600"`
}

type CreateResp struct {
	UploadID    uuid.UUID `json:"upload_id"`
	TotalChunks int       `json:"total_chunks"`
	ChunkURLs   []string  `json:"chunk_urls"`
	CompleteURL string    `json:"complete_url"`
	ExpiresAt   time.Time `json:"expires_at"`
}

type ChunkReq struct {
	Size     int    `json:"size" validate:"required,gte=0"`
	Checksum string `json:"checksum" validate:"required,len=64"`
}

type CompleteReq struct {
	FinalChecksum string `json:"final_checksum" validate:"omitempty,len=64"`
}

type ResumeResp struct {
	Upload   model.Upload `json:"upload"`
	Missing  []int        `json:"missing_chunks"`
	ChunkURL []string     `json:"chunk_urls,omitempty"`
}
