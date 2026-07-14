package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	sqlc "github.com/MobasirSarkar/MediaEngine/internal/db/sqlc"
	errs "github.com/MobasirSarkar/MediaEngine/internal/err"
	"github.com/MobasirSarkar/MediaEngine/internal/model"
)

type Uploads struct {
	q *sqlc.Queries
}

func NewUploads(q *sqlc.Queries) *Uploads { return &Uploads{q: q} }

func (r *Uploads) Create(ctx context.Context, u model.Upload) error {
	_, err := r.q.CreateUpload(ctx, sqlc.CreateUploadParams{
		ID:          u.ID,
		OwnerID:     u.OwnerID,
		Filename:    u.Filename,
		ContentType: u.ContentType,
		TotalSize:   u.TotalSize,
		ChunkSize:   int32(u.ChunkSize),
		TotalChunks: int32(u.TotalChunks),
		Status:      string(u.Status),
		Bucket:      u.Bucket,
		Key:         u.Key,
		ExpiresAt:   toPtrTime(u.ExpiresAt),
	})
	if err != nil {
		return fmt.Errorf("store: create upload: %w", err)
	}
	return nil
}

func (r *Uploads) Get(ctx context.Context, id uuid.UUID) (model.Upload, error) {
	row, err := r.q.GetUpload(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.Upload{}, errs.ErrNotFound
		}
		return model.Upload{}, fmt.Errorf("store: get upload: %w", err)
	}
	return uploadFromRow(row), nil
}

func (r *Uploads) UpdateStatus(ctx context.Context, id uuid.UUID, status model.UploadStatus) (model.Upload, error) {
	row, err := r.q.UpdateUploadStatus(ctx, sqlc.UpdateUploadStatusParams{
		ID:     id,
		Status: string(status),
	})
	if err != nil {
		return model.Upload{}, fmt.Errorf("store: update status: %w", err)
	}
	return uploadFromRow(row), nil
}

func (r *Uploads) AddBytes(ctx context.Context, id uuid.UUID, by int64) (model.Upload, error) {
	row, err := r.q.IncrementReceivedBytes(ctx, sqlc.IncrementReceivedBytesParams{
		ID:            id,
		ReceivedBytes: by,
	})
	if err != nil {
		return model.Upload{}, fmt.Errorf("store: inc bytes: %w", err)
	}
	return uploadFromRow(row), nil
}

func (r *Uploads) SetChecksum(ctx context.Context, id uuid.UUID, sum string) error {
	_, err := r.q.SetUploadChecksum(ctx, sqlc.SetUploadChecksumParams{
		ID:             id,
		ChecksumSha256: toPtrStr(sum),
		Status:         string(model.UploadCompleted),
	})
	if err != nil {
		return fmt.Errorf("store: set checksum: %w", err)
	}
	return nil
}

func (r *Uploads) AppendChunk(ctx context.Context, uploadID uuid.UUID, chunk model.Chunk) error {
	_, err := r.q.AppendChunk(ctx, sqlc.AppendChunkParams{
		UploadID: uploadID,
		ChunkNo:  int32(chunk.ChunkNo),
		Size:     int32(chunk.Size),
		Checksum: chunk.Checksum,
	})
	if err != nil {
		return fmt.Errorf("store: append chunk: %w", err)
	}
	return nil
}

func (r *Uploads) ListChunks(ctx context.Context, uploadID uuid.UUID) ([]model.Chunk, error) {
	rows, err := r.q.ListChunks(ctx, uploadID)
	if err != nil {
		return nil, fmt.Errorf("store: list chunks: %w", err)
	}
	out := make([]model.Chunk, 0, len(rows))
	for _, r := range rows {
		out = append(out, model.Chunk{
			UploadID:   r.UploadID,
			ChunkNo:    int(r.ChunkNo),
			Size:       int(r.Size),
			Checksum:   r.Checksum,
			ReceivedAt: r.ReceivedAt,
		})
	}
	return out, nil
}

func (r *Uploads) ChunkCount(ctx context.Context, uploadID uuid.UUID) (int, error) {
	n, err := r.q.CountChunks(ctx, uploadID)
	if err != nil {
		return 0, fmt.Errorf("store: count chunks: %w", err)
	}
	return int(n), nil
}

func uploadFromRow(u sqlc.Upload) model.Upload {
	return model.Upload{
		ID:             u.ID,
		OwnerID:        u.OwnerID,
		Filename:       u.Filename,
		ContentType:    u.ContentType,
		TotalSize:      u.TotalSize,
		ChunkSize:      int(u.ChunkSize),
		TotalChunks:    int(u.TotalChunks),
		ReceivedBytes:  u.ReceivedBytes,
		Status:         model.UploadStatus(u.Status),
		Bucket:         u.Bucket,
		Key:            u.Key,
		ChecksumSHA256: fromPtrStr(u.ChecksumSha256),
		CreatedAt:      u.CreatedAt,
		UpdatedAt:      u.UpdatedAt,
		ExpiresAt:      fromPtrTime(u.ExpiresAt),
	}
}
