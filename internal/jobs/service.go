package jobs

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	errs "github.com/MobasirSarkar/MediaEngine/internal/err"
	"github.com/MobasirSarkar/MediaEngine/internal/events"
	"github.com/MobasirSarkar/MediaEngine/internal/model"
)

type Repo interface {
	Create(ctx context.Context, j model.Job) error
	Get(ctx context.Context, id uuid.UUID) (model.Job, error)
	GetByUpload(ctx context.Context, uploadID uuid.UUID) (model.Job, error)
	UpdateStatus(ctx context.Context, id uuid.UUID, status model.JobStatus, code, msg string, retries int) (model.Job, error)
	CreateTask(ctx context.Context, t model.Task) error
	ListTasks(ctx context.Context, jobID uuid.UUID) ([]model.Task, error)
}

type Bus interface {
	Publish(ctx context.Context, subject string, payload []byte, headers map[string]string) error
}

type Service struct {
	repo Repo
	bus  Bus
}

type ServiceOption func(*Service)

func NewService(repo Repo, bus Bus) *Service {
	return &Service{repo: repo, bus: bus}
}

func (s *Service) Get(ctx context.Context, id uuid.UUID) (model.Job, error) {
	return s.repo.Get(ctx, id)
}

func (s *Service) GetByUpload(ctx context.Context, uploadID uuid.UUID) (model.Job, error) {
	return s.repo.GetByUpload(ctx, uploadID)
}

func (s *Service) ListTasks(ctx context.Context, jobID uuid.UUID) ([]model.Task, error) {
	return s.repo.ListTasks(ctx, jobID)
}

// CreateForUpload enqueues a job + its task set after upload completion.
func (s *Service) CreateForUpload(ctx context.Context, uploadID uuid.UUID, key string) (model.Job, error) {
	jobID := uuid.New()
	j := model.Job{
		ID:       jobID,
		UploadID: uploadID,
		Status:   model.JobQueued,
	}
	if err := s.repo.Create(ctx, j); err != nil {
		return model.Job{}, errs.Wrap(err, errs.ErrInternal, "create job")
	}
	tasks := []model.Task{
		{ID: uuid.New(), JobID: jobID, Kind: model.TaskMetadata, Status: model.TaskPending},
		{ID: uuid.New(), JobID: jobID, Kind: model.TaskThumbnail, Status: model.TaskPending},
		{ID: uuid.New(), JobID: jobID, Kind: model.TaskTranscode, Status: model.TaskPending},
	}
	for _, t := range tasks {
		if err := s.repo.CreateTask(ctx, t); err != nil {
			return model.Job{}, errs.Wrap(err, errs.ErrInternal, "create task")
		}
		payload, _ := json.Marshal(map[string]any{
			"task_id":   t.ID.String(),
			"job_id":    jobID.String(),
			"upload_id": uploadID.String(),
			"key":       key,
			"kind":      t.Kind,
		})
		subject := subjectFor(t.Kind)
		if err := s.bus.Publish(ctx, subject, payload, map[string]string{
			events.HdrUploadID: uploadID.String(),
			events.HdrJobID:    jobID.String(),
			events.HdrTaskID:   t.ID.String(),
		}); err != nil {
			return model.Job{}, errs.Wrap(err, errs.ErrInternal, "publish task")
		}
	}
	return j, nil
}

func subjectFor(k model.TaskKind) string {
	switch k {
	case model.TaskMetadata:
		return events.JobCreated + ".metadata"
	case model.TaskThumbnail:
		return events.JobCreated + ".thumbnail"
	case model.TaskTranscode:
		return events.JobCreated + ".transcode"
	default:
		return fmt.Sprintf("pipeline.task.%s", k)
	}
}
