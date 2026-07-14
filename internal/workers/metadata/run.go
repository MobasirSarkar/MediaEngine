package metadata

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"go.uber.org/zap"

	errs "github.com/MobasirSarkar/MediaEngine/internal/err"
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
}

type Config struct {
	Bucket string
}

type Worker struct {
	repo   Repo
	bus    Events
	bucket string
	store  Storage
}

func New(cfg Config, repo Repo, bus Events, store Storage) *Worker {
	return &Worker{repo: repo, bus: bus, bucket: cfg.Bucket, store: store}
}

type payload struct {
	UploadID string `json:"upload_id"`
	Key      string `json:"key"`
}

func (w *Worker) Run(ctx context.Context) error {
	return w.bus.Subscribe(ctx, events.JobCreated+".metadata", "worker-metadata", func(ctx context.Context, msg events.Message) error {
		logger := logpkg.From(ctx)
		taskID, err := uuid.Parse(msg.Headers[events.HdrTaskID])
		if err != nil {
			logger.Error("metadata worker: invalid task id in header", zap.Error(err))
			return nil
		}
		logger.Info("metadata worker: received task", zap.String("task_id", taskID.String()))
		if err := w.run(ctx, taskID, msg); err != nil {
			logger.Error("metadata worker: task failed", zap.String("task_id", taskID.String()), zap.Error(err))
			_ = w.repo.FailTask(ctx, taskID, err.Error())
		} else {
			logger.Info("metadata worker: task completed successfully", zap.String("task_id", taskID.String()))
		}
		return nil
	})
}

func (w *Worker) run(ctx context.Context, taskID uuid.UUID, msg events.Message) error {
	if err := w.repo.StartTask(ctx, taskID); err != nil {
		return fmt.Errorf("start: %w", err)
	}
	var p payload
	_ = json.Unmarshal(msg.Payload, &p)
	if p.UploadID == "" {
		return errs.New(errs.ErrInvalid, "missing upload_id in payload", nil)
	}
	key := p.Key
	if key == "" {
		key = "uploads/" + p.UploadID + "/source"
	}
	result, err := w.ffprobe(ctx, key)
	if err != nil {
		logger := logpkg.From(ctx)
		logger.Warn("ffprobe failed; recording unavailable", zap.Error(err), zap.String("key", key))
		result = map[string]any{"source": key, "available": false}
	}
	b, _ := json.Marshal(result)
	return w.repo.CompleteTask(ctx, taskID, b)
}

func (w *Worker) ffprobe(ctx context.Context, key string) (map[string]any, error) {
	urlStr, err := w.store.PresignGet(ctx, w.bucket, key)
	if err != nil {
		return nil, fmt.Errorf("presign: %w", err)
	}
	args := []string{"-v", "quiet", "-print_format", "json", "-show_streams", "-show_format", urlStr}
	out, err := exec.CommandContext(ctx, "ffprobe", args...).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("ffprobe: %w", err)
	}
	return parseFFprobe(string(out)), nil
}

func parseFFprobe(s string) map[string]any {
	out := map[string]any{}
	var root map[string]any
	if err := json.Unmarshal([]byte(s), &root); err != nil {
		return out
	}
	if streams, ok := root["streams"].([]any); ok {
		for _, raw := range streams {
			m, _ := raw.(map[string]any)
			if m == nil {
				continue
			}
			codecType, _ := m["codec_type"].(string)
			switch codecType {
			case "video":
				if v, ok := m["width"].(float64); ok {
					out["width"] = int(v)
				}
				if v, ok := m["height"].(float64); ok {
					out["height"] = int(v)
				}
				if v, ok := m["duration"].(string); ok {
					if n, err := strconv.ParseFloat(v, 64); err == nil {
						out["duration_seconds"] = n
					}
				}
				out["codec"] = m["codec_name"]
				if v, ok := m["r_frame_rate"].(string); ok {
					out["fps"] = parseFrameRate(v)
				}
			case "audio":
				out["has_audio"] = true
			}
		}
	}
	if fmt_, ok := root["format"].(map[string]any); ok {
		if v, ok := fmt_["bit_rate"].(string); ok {
			if n, err := strconv.ParseInt(v, 10, 64); err == nil {
				out["bitrate"] = n
			}
		}
		if v, ok := fmt_["size"].(string); ok {
			if n, err := strconv.ParseInt(v, 10, 64); err == nil {
				out["file_size"] = n
			}
		}
	}
	return out
}

func parseFrameRate(r string) float64 {
	parts := strings.SplitN(r, "/", 2)
	if len(parts) == 2 {
		num, _ := strconv.ParseFloat(parts[0], 64)
		den, _ := strconv.ParseFloat(parts[1], 64)
		if den > 0 {
			return num / den
		}
		return 0
	}
	n, err := strconv.ParseFloat(r, 64)
	if err != nil {
		return 0
	}
	return n
}
