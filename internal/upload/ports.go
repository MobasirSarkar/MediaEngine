package upload

import (
	"context"

	"github.com/google/uuid"

	"github.com/MobasirSarkar/MediaEngine/internal/model"
	"github.com/MobasirSarkar/MediaEngine/internal/storage"
)

type Repo interface {
	Create(ctx context.Context, u model.Upload) error
	Get(ctx context.Context, id uuid.UUID) (model.Upload, error)
	UpdateStatus(ctx context.Context, id uuid.UUID, status model.UploadStatus) (model.Upload, error)
	AddBytes(ctx context.Context, id uuid.UUID, by int64) (model.Upload, error)
	SetChecksum(ctx context.Context, id uuid.UUID, sum string) error
	AppendChunk(ctx context.Context, uploadID uuid.UUID, chunk model.Chunk) error
	ListChunks(ctx context.Context, uploadID uuid.UUID) ([]model.Chunk, error)
	ChunkCount(ctx context.Context, uploadID uuid.UUID) (int, error)
}

type ObjectStore interface {
	PresignPut(ctx context.Context, bucket, key string, size int64) (string, error)
	Compose(ctx context.Context, bucket, dst string, sources []string) error
	Stat(ctx context.Context, bucket, key string) (storage.Info, error)
}

type Bus interface {
	Publish(ctx context.Context, subject string, payload []byte, headers map[string]string) error
}
