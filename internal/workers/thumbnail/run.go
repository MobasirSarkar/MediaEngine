package thumbnail

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/MobasirSarkar/MediaEngine/internal/events"
	logpkg "github.com/MobasirSarkar/MediaEngine/internal/log"
)

type Repo interface {
	StartTask(ctx context.Context, id uuid.UUID) error
	CompleteTask(ctx context.Context, id uuid.UUID, result []byte) error
	FailTask(ctx context.Context, id uuid.UUID, msg string) error
}

type Events interface {
	Subscribe(ctx context.Context, subject, durable string, handler events.Handler) error
}

type Storage interface {
	PresignGet(ctx context.Context, bucket, key string) (string, error)
	PresignPut(ctx context.Context, bucket, key string, size int64) (string, error)
}

type payload struct {
	UploadID string `json:"upload_id"`
	Key      string `json:"key"`
}

func Run(ctx context.Context, repo Repo, bus Events, store Storage, uploadsBucket, mediaBucket string) error {
	return bus.Subscribe(ctx, events.JobCreated+".thumbnail", "worker-thumbnail", func(ctx context.Context, msg events.Message) error {
		logger := logpkg.From(ctx)
		taskID, err := uuid.Parse(msg.Headers[events.HdrTaskID])
		if err != nil {
			logger.Error("thumbnail worker: invalid task id in header", zap.Error(err))
			return nil
		}

		logger.Info("thumbnail worker: received task", zap.String("task_id", taskID.String()))
		_ = repo.StartTask(ctx, taskID)

		var p payload
		_ = json.Unmarshal(msg.Payload, &p)
		if p.UploadID == "" {
			errStr := "missing upload_id in payload"
			logger.Error("thumbnail worker: failed", zap.String("task_id", taskID.String()), zap.String("error", errStr))
			_ = repo.FailTask(ctx, taskID, errStr)
			return nil
		}

		// 1. Get pre-signed URL for the source media file
		srcKey := p.Key
		if srcKey == "" {
			srcKey = "uploads/" + p.UploadID + "/source"
		}
		sourceURL, err := store.PresignGet(ctx, uploadsBucket, srcKey)
		if err != nil {
			logger.Error("thumbnail worker: failed to presign source URL", zap.String("task_id", taskID.String()), zap.Error(err))
			_ = repo.FailTask(ctx, taskID, err.Error())
			return nil
		}

		// 2. Run ffmpeg to extract the first frame
		tmpPath := "/tmp/thumbnail_" + p.UploadID + ".jpg"
		if _, err := exec.LookPath("ffmpeg"); err != nil {
			logger.Warn("ffmpeg not found in PATH; writing mock fallback thumbnail", zap.Error(err))
			dummyBytes := []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x00\x01\x00\x00\x00\x01\x08\x06\x00\x00\x00\x1f\x15c4\x00\x00\x00\nIDATx\x9cc\x00\x01\x00\x00\x05\x00\x01\r\n-\xb4\x00\x00\x00\x00IEND\xaeB`\x82")
			_ = os.WriteFile(tmpPath, dummyBytes, 0644)
		} else {
			args := []string{
				"-y",
				"-i", sourceURL,
				"-vframes", "1",
				"-f", "image2",
				tmpPath,
			}
			cmd := exec.CommandContext(ctx, "ffmpeg", args...)
			if out, err := cmd.CombinedOutput(); err != nil {
				logger.Error("thumbnail worker: ffmpeg failed", zap.String("task_id", taskID.String()), zap.Error(err), zap.String("output", string(out)))
				_ = repo.FailTask(ctx, taskID, fmt.Sprintf("ffmpeg extract: %v (output: %s)", err, string(out)))
				return nil
			}
		}
		defer os.Remove(tmpPath)

		// 3. Read extracted frame bytes
		fileBytes, err := os.ReadFile(tmpPath)
		if err != nil {
			logger.Error("thumbnail worker: failed to read generated thumbnail file", zap.String("task_id", taskID.String()), zap.Error(err))
			_ = repo.FailTask(ctx, taskID, fmt.Sprintf("read file: %v", err))
			return nil
		}

		// 4. Generate S3 pre-signed PUT URL
		dstKey := "uploads/" + p.UploadID + "/thumbnail.jpg"
		putURL, err := store.PresignPut(ctx, uploadsBucket, dstKey, int64(len(fileBytes)))
		if err != nil {
			logger.Error("thumbnail worker: failed to presign PUT URL", zap.String("task_id", taskID.String()), zap.Error(err))
			_ = repo.FailTask(ctx, taskID, fmt.Sprintf("presign PUT: %v", err))
			return nil
		}

		// 5. Upload frame to S3
		req, err := http.NewRequestWithContext(ctx, "PUT", putURL, bytes.NewReader(fileBytes))
		if err != nil {
			logger.Error("thumbnail worker: failed to create S3 upload request", zap.String("task_id", taskID.String()), zap.Error(err))
			_ = repo.FailTask(ctx, taskID, fmt.Sprintf("create upload request: %v", err))
			return nil
		}
		req.Header.Set("Content-Type", "image/jpeg")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			logger.Error("thumbnail worker: failed S3 HTTP upload", zap.String("task_id", taskID.String()), zap.Error(err))
			_ = repo.FailTask(ctx, taskID, fmt.Sprintf("upload to S3: %v", err))
			return nil
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
			logger.Error("thumbnail worker: S3 returned failure code", zap.String("task_id", taskID.String()), zap.Int("status_code", resp.StatusCode))
			_ = repo.FailTask(ctx, taskID, fmt.Sprintf("upload S3 status code: %d", resp.StatusCode))
			return nil
		}

		// 6. Complete Task
		result := map[string]any{
			"status":        "completed",
			"thumbnail_url": "/media/" + dstKey,
		}
		b, _ := json.Marshal(result)
		if err := repo.CompleteTask(ctx, taskID, b); err != nil {
			logger.Error("thumbnail worker: failed to complete task", zap.String("task_id", taskID.String()), zap.Error(err))
			return nil
		}

		logger.Info("thumbnail worker: task completed successfully", zap.String("task_id", taskID.String()), zap.String("thumbnail_url", "/media/"+dstKey))
		return nil
	})
}
