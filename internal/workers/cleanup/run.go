package cleanup

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/MobasirSarkar/MediaEngine/internal/events"
	logpkg "github.com/MobasirSarkar/MediaEngine/internal/log"
)

type Events interface {
	Subscribe(ctx context.Context, subject, durable string, handler events.Handler) error
}

type Storage interface {
	RemovePrefix(ctx context.Context, bucket, prefix string) error
}

type payload struct {
	UploadID string `json:"upload_id"`
}

func Run(ctx context.Context, bus Events, store Storage, bucket string) error {
	logger := logpkg.From(ctx)

	// 1. Subscribe to Upload Completed to clean up intermediate chunks
	err := bus.Subscribe(ctx, events.UploadCompleted, "cleanup-completed", func(ctx context.Context, msg events.Message) error {
		var p payload
		if err := json.Unmarshal(msg.Payload, &p); err != nil {
			logger.Error("cleanup worker: failed to decode complete payload", zap.Error(err))
			return nil
		}
		if p.UploadID == "" {
			return nil
		}

		chunksPrefix := fmt.Sprintf("uploads/%s/chunks/", p.UploadID)
		logger.Info("cleanup worker: composition finished, removing source chunks", zap.String("upload_id", p.UploadID), zap.String("prefix", chunksPrefix))

		if err := store.RemovePrefix(ctx, bucket, chunksPrefix); err != nil {
			logger.Error("cleanup worker: failed to delete chunks", zap.String("upload_id", p.UploadID), zap.Error(err))
			return nil
		}

		logger.Info("cleanup worker: chunks removed successfully", zap.String("upload_id", p.UploadID))
		return nil
	})
	if err != nil {
		return fmt.Errorf("cleanup: subscribe completed: %w", err)
	}

	// 2. Subscribe to Upload Canceled to clean up the entire upload folder
	err = bus.Subscribe(ctx, events.UploadCanceled, "cleanup-canceled", func(ctx context.Context, msg events.Message) error {
		var p payload
		if err := json.Unmarshal(msg.Payload, &p); err != nil {
			logger.Error("cleanup worker: failed to decode cancel payload", zap.Error(err))
			return nil
		}
		if p.UploadID == "" {
			return nil
		}

		uploadPrefix := fmt.Sprintf("uploads/%s/", p.UploadID)
		logger.Info("cleanup worker: upload canceled, purging folder", zap.String("upload_id", p.UploadID), zap.String("prefix", uploadPrefix))

		if err := store.RemovePrefix(ctx, bucket, uploadPrefix); err != nil {
			logger.Error("cleanup worker: failed to purge folder", zap.String("upload_id", p.UploadID), zap.Error(err))
			return nil
		}

		logger.Info("cleanup worker: upload folder purged successfully", zap.String("upload_id", p.UploadID))
		return nil
	})
	if err != nil {
		return fmt.Errorf("cleanup: subscribe canceled: %w", err)
	}

	// 3. Periodic clean up routine to sweep abandoned uploads (>24 hours) from database & S3
	// Note: Can be extended in a production environment as a recurring cron job.
	logger.Info("cleanup worker: listeners initialized successfully")
	return nil
}
type modelTask struct {
	ID       uuid.UUID
	JobID    uuid.UUID
	Kind     string
	Status   string
	Result   []byte
	ErrorMsg string
}
