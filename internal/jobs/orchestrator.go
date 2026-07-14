package jobs

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"go.uber.org/zap"

	errs "github.com/MobasirSarkar/MediaEngine/internal/err"
	"github.com/MobasirSarkar/MediaEngine/internal/events"
	"github.com/MobasirSarkar/MediaEngine/internal/hub"
	logpkg "github.com/MobasirSarkar/MediaEngine/internal/log"
)

type Orchestrator struct {
	svc *Service
	bus Events
	hub *hub.Hub
}

type Events interface {
	Subscribe(ctx context.Context, subject string, durable string, handler events.Handler) error
}

func NewOrchestrator(svc *Service, bus Events, h *hub.Hub) *Orchestrator {
	return &Orchestrator{svc: svc, bus: bus, hub: h}
}

type uploadCompletedPayload struct {
	UploadID string `json:"upload_id"`
	Key      string `json:"key"`
}

func (o *Orchestrator) Run(ctx context.Context) error {
	return o.bus.Subscribe(ctx, events.UploadCompleted, "upload-completed", func(ctx context.Context, msg events.Message) error {
		logger := logpkg.From(ctx)
		var p uploadCompletedPayload
		if err := json.Unmarshal(msg.Payload, &p); err != nil {
			logger.Error("orchestrator: decode", zap.Error(err))
			return nil
		}
		uid, err := uuid.Parse(p.UploadID)
		if err != nil {
			logger.Error("orchestrator: bad upload id", zap.String("val", p.UploadID))
			return nil
		}
		job, err := o.svc.CreateForUpload(ctx, uid, p.Key)
		if err != nil {
			if errs.Is(err, errs.ErrInternal) {
				return err
			}
			logger.Error("orchestrator: create job", zap.Error(err))
			return nil
		}
		o.hub.Publish(job.ID.String(), hub.Event{Type: "JobCreated", Data: map[string]any{"job_id": job.ID.String()}})
		return nil
	})
}
