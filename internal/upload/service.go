package upload

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	errs "github.com/MobasirSarkar/MediaEngine/internal/err"
	"github.com/MobasirSarkar/MediaEngine/internal/events"
	"github.com/MobasirSarkar/MediaEngine/internal/model"
)

type Config struct {
	SessionTTL time.Duration
	Bucket     string
}

type Service struct {
	cfg   Config
	repo  Repo
	store ObjectStore
	bus   Bus
}

func NewService(cfg Config, repo Repo, store ObjectStore, bus Bus) *Service {
	return &Service{cfg: cfg, repo: repo, store: store, bus: bus}
}

func (s *Service) Create(ctx context.Context, req CreateReq) (CreateResp, error) {
	totalChunks := int((req.TotalSize + req.ChunkSize - 1) / req.ChunkSize)
	if totalChunks <= 0 || totalChunks > 10000 {
		return CreateResp{}, errs.Wrap(errs.ErrInvalid, errs.ErrInvalid, "chunk count out of range")
	}
	id := uuid.New()
	key := fmt.Sprintf("uploads/%s/%s", id, req.Filename)
	now := time.Now().UTC()
	u := model.Upload{
		ID:          id,
		OwnerID:     req.OwnerID,
		Filename:    req.Filename,
		ContentType: req.ContentType,
		TotalSize:   req.TotalSize,
		ChunkSize:   int(req.ChunkSize),
		TotalChunks: totalChunks,
		Status:      model.UploadCreated,
		Bucket:      s.cfg.Bucket,
		Key:         key,
		CreatedAt:   now,
		UpdatedAt:   now,
		ExpiresAt:   now.Add(s.cfg.SessionTTL),
	}
	if err := s.repo.Create(ctx, u); err != nil {
		return CreateResp{}, errs.Wrap(err, errs.ErrInternal, "create upload")
	}
	urls := make([]string, 0, totalChunks)
	for i := 0; i < totalChunks; i++ {
		chunkKey := fmt.Sprintf("%s/chunks/%05d", key, i)
		presigned, err := s.store.PresignPut(ctx, s.cfg.Bucket, chunkKey, req.ChunkSize)
		if err != nil {
			return CreateResp{}, errs.Wrap(err, errs.ErrInternal, "presign chunk")
		}
		urls = append(urls, presigned)
	}
	return CreateResp{
		UploadID:    id,
		TotalChunks: totalChunks,
		ChunkURLs:   urls,
		CompleteURL: fmt.Sprintf("/uploads/%s/complete", id),
		ExpiresAt:   u.ExpiresAt,
	}, nil
}

func (s *Service) AppendChunk(ctx context.Context, uploadID uuid.UUID, n int, req ChunkReq) (model.Upload, error) {
	u, err := s.repo.Get(ctx, uploadID)
	if err != nil {
		return model.Upload{}, err
	}
	if u.Status == model.UploadCompleted || u.Status == model.UploadCanceled || u.Status == model.UploadExpired {
		return model.Upload{}, errs.Wrap(errs.ErrConflict, errs.ErrConflict, "upload not active")
	}
	if n < 0 || n >= u.TotalChunks {
		return model.Upload{}, errs.Wrap(errs.ErrInvalid, errs.ErrInvalid, "chunk_no out of range")
	}
	chunkKey := fmt.Sprintf("%s/chunks/%05d", u.Key, n)
	info, err := s.store.Stat(ctx, s.cfg.Bucket, chunkKey)
	if err != nil {
		if errs.Is(err, errs.ErrNotFound) {
			return model.Upload{}, errs.Wrap(errs.ErrConflict, errs.ErrConflict, "chunk not uploaded yet")
		}
		return model.Upload{}, errs.Wrap(err, errs.ErrInternal, "stat chunk")
	}
	if info.Size != int64(req.Size) {
		return model.Upload{}, errs.Wrap(errs.ErrConflict, errs.ErrConflict, "chunk size mismatch")
	}
	chunk := model.Upload{}
	_ = chunk
	c := model.Chunk{UploadID: uploadID, ChunkNo: n, Size: req.Size, Checksum: req.Checksum}
	if err := s.repo.AppendChunk(ctx, uploadID, c); err != nil {
		return model.Upload{}, errs.Wrap(err, errs.ErrInternal, "append chunk")
	}
	u2, err := s.repo.AddBytes(ctx, uploadID, int64(req.Size))
	if err != nil {
		return model.Upload{}, errs.Wrap(err, errs.ErrInternal, "add bytes")
	}
	if u2.Status == model.UploadCreated {
		u2, err = s.repo.UpdateStatus(ctx, uploadID, model.UploadUploading)
		if err != nil {
			return model.Upload{}, errs.Wrap(err, errs.ErrInternal, "update status")
		}
	}
	return u2, nil
}

func (s *Service) Complete(ctx context.Context, uploadID uuid.UUID, req CompleteReq) (model.Upload, error) {
	u, err := s.repo.Get(ctx, uploadID)
	if err != nil {
		return model.Upload{}, err
	}
	if u.Status == model.UploadCompleted {
		return u, nil
	}
	if u.Status != model.UploadUploading && u.Status != model.UploadCreated {
		return model.Upload{}, errs.Wrap(errs.ErrConflict, errs.ErrConflict, "upload not in progress")
	}
	if u.ReceivedBytes != u.TotalSize {
		return model.Upload{}, errs.Wrap(errs.ErrConflict, errs.ErrConflict, "incomplete bytes received")
	}
	count, err := s.repo.ChunkCount(ctx, uploadID)
	if err != nil {
		return model.Upload{}, errs.Wrap(err, errs.ErrInternal, "count chunks")
	}
	if count != u.TotalChunks {
		return model.Upload{}, errs.Wrap(errs.ErrConflict, errs.ErrConflict, "missing chunks")
	}
	sources := make([]string, 0, u.TotalChunks)
	for i := 0; i < u.TotalChunks; i++ {
		sources = append(sources, fmt.Sprintf("%s/chunks/%05d", u.Key, i))
	}
	if err := s.store.Compose(ctx, s.cfg.Bucket, u.Key, sources); err != nil {
		return model.Upload{}, errs.Wrap(err, errs.ErrInternal, "compose object")
	}
	if err := s.repo.SetChecksum(ctx, uploadID, req.FinalChecksum); err != nil {
		return model.Upload{}, errs.Wrap(err, errs.ErrInternal, "set checksum")
	}
	updated, err := s.repo.Get(ctx, uploadID)
	if err != nil {
		return model.Upload{}, errs.Wrap(err, errs.ErrInternal, "reload upload")
	}
	payload, _ := json.Marshal(map[string]any{
		"upload_id": updated.ID.String(),
		"key":       updated.Key,
		"size":      updated.TotalSize,
	})
	_ = s.bus.Publish(ctx, events.UploadCompleted, payload, map[string]string{
		events.HdrUploadID: uploadID.String(),
	})
	return updated, nil
}

func (s *Service) Cancel(ctx context.Context, uploadID uuid.UUID) error {
	u, err := s.repo.UpdateStatus(ctx, uploadID, model.UploadCanceled)
	if err != nil {
		return err
	}
	payload, _ := json.Marshal(map[string]any{
		"upload_id": u.ID.String(),
		"key":       u.Key,
	})
	_ = s.bus.Publish(ctx, events.UploadCanceled, payload, map[string]string{
		events.HdrUploadID: uploadID.String(),
	})
	return nil
}

func (s *Service) Resume(ctx context.Context, uploadID uuid.UUID) (ResumeResp, error) {
	u, err := s.repo.Get(ctx, uploadID)
	if err != nil {
		return ResumeResp{}, err
	}
	chunks, err := s.repo.ListChunks(ctx, uploadID)
	if err != nil {
		return ResumeResp{}, errs.Wrap(err, errs.ErrInternal, "list chunks")
	}
	received := make(map[int]bool, len(chunks))
	for _, c := range chunks {
		received[c.ChunkNo] = true
	}
	missing := make([]int, 0)
	urls := make([]string, 0)
	for i := 0; i < u.TotalChunks; i++ {
		if received[i] {
			continue
		}
		missing = append(missing, i)
		chunkKey := fmt.Sprintf("%s/chunks/%05d", u.Key, i)
		presigned, err := s.store.PresignPut(ctx, s.cfg.Bucket, chunkKey, int64(u.ChunkSize))
		if err != nil {
			return ResumeResp{}, errs.Wrap(err, errs.ErrInternal, "presign")
		}
		urls = append(urls, presigned)
	}
	return ResumeResp{Upload: u, Missing: missing, ChunkURL: urls}, nil
}
