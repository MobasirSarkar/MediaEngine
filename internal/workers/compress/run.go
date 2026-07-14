package compress

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/MobasirSarkar/MediaEngine/internal/events"
	logpkg "github.com/MobasirSarkar/MediaEngine/internal/log"
)

type Repo interface {
	StartTask(ctx context.Context, id uuid.UUID) error
	CompleteTask(ctx context.Context, id uuid.UUID, result []byte) error
}

type Events interface {
	Subscribe(ctx context.Context, subject, durable string, handler events.Handler) error
}

func Run(ctx context.Context, repo Repo, bus Events) error {
	return bus.Subscribe(ctx, "pipeline.task.compress", "worker-compress", func(ctx context.Context, msg events.Message) error {
		logger := logpkg.From(ctx)
		taskID, err := uuid.Parse(msg.Headers[events.HdrTaskID])
		if err != nil {
			logger.Error("compress worker: invalid task id in header", zap.Error(err))
			return nil
		}

		logger.Info("compress worker: received task", zap.String("task_id", taskID.String()))
		_ = repo.StartTask(ctx, taskID)

		result := map[string]any{"status": "skipped", "reason": "compress worker not implemented"}
		b, _ := json.Marshal(result)

		if err := repo.CompleteTask(ctx, taskID, b); err != nil {
			logger.Error("compress worker: failed to complete task", zap.String("task_id", taskID.String()), zap.Error(err))
			return nil
		}

		logger.Info("compress worker: task completed successfully (skipped stub)", zap.String("task_id", taskID.String()))
		return nil
	})
}
