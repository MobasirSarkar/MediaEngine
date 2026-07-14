package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"go.uber.org/zap"

	sqlc "github.com/MobasirSarkar/MediaEngine/internal/db/sqlc"
	errs "github.com/MobasirSarkar/MediaEngine/internal/err"
	logpkg "github.com/MobasirSarkar/MediaEngine/internal/log"
	"github.com/MobasirSarkar/MediaEngine/internal/model"
)

type Jobs struct {
	q *sqlc.Queries
}

func NewJobs(q *sqlc.Queries) *Jobs { return &Jobs{q: q} }

func (r *Jobs) Create(ctx context.Context, j model.Job) error {
	_, err := r.q.CreateJob(ctx, sqlc.CreateJobParams{
		ID:       j.ID,
		UploadID: j.UploadID,
		Status:   string(j.Status),
	})
	if err != nil {
		return fmt.Errorf("store: create job: %w", err)
	}
	return nil
}

func (r *Jobs) Get(ctx context.Context, id uuid.UUID) (model.Job, error) {
	row, err := r.q.GetJob(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.Job{}, errs.ErrNotFound
		}
		return model.Job{}, fmt.Errorf("store: get job: %w", err)
	}
	return jobFromRow(row), nil
}

func (r *Jobs) GetByUpload(ctx context.Context, uploadID uuid.UUID) (model.Job, error) {
	row, err := r.q.GetJobByUpload(ctx, uploadID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.Job{}, errs.ErrNotFound
		}
		return model.Job{}, fmt.Errorf("store: get job by upload: %w", err)
	}
	return jobFromRow(row), nil
}

func (r *Jobs) UpdateStatus(ctx context.Context, id uuid.UUID, status model.JobStatus, errCode, errMsg string, retries int) (model.Job, error) {
	row, err := r.q.UpdateJobStatus(ctx, sqlc.UpdateJobStatusParams{
		ID:        id,
		Status:    string(status),
		ErrorCode: toPtrStr(errCode),
		ErrorMsg:  toPtrStr(errMsg),
		Retries:   int32(retries),
	})
	if err != nil {
		return model.Job{}, fmt.Errorf("store: update job: %w", err)
	}
	return jobFromRow(row), nil
}

func (r *Jobs) CreateTask(ctx context.Context, t model.Task) error {
	_, err := r.q.CreateTask(ctx, sqlc.CreateTaskParams{
		ID:      t.ID,
		JobID:   t.JobID,
		Kind:    string(t.Kind),
		Status:  string(t.Status),
		Payload: t.Payload,
	})
	if err != nil {
		return fmt.Errorf("store: create task: %w", err)
	}
	return nil
}

func (r *Jobs) GetTask(ctx context.Context, id uuid.UUID) (model.Task, error) {
	row, err := r.q.GetTask(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.Task{}, errs.ErrNotFound
		}
		return model.Task{}, fmt.Errorf("store: get task: %w", err)
	}
	return taskFromRow(row), nil
}

func (r *Jobs) ListTasks(ctx context.Context, jobID uuid.UUID) ([]model.Task, error) {
	rows, err := r.q.ListTasksByJob(ctx, jobID)
	if err != nil {
		return nil, fmt.Errorf("store: list tasks: %w", err)
	}
	out := make([]model.Task, 0, len(rows))
	for _, r := range rows {
		out = append(out, taskFromRow(r))
	}
	return out, nil
}

func (r *Jobs) StartTask(ctx context.Context, id uuid.UUID) error {
	task, err := r.GetTask(ctx, id)
	if err != nil {
		return err
	}
	if _, err := r.q.UpdateTaskStarted(ctx, id); err != nil {
		return fmt.Errorf("store: start task: %w", err)
	}
	_, _ = r.UpdateStatus(ctx, task.JobID, model.JobRunning, "", "", 0)
	return nil
}

func (r *Jobs) CompleteTask(ctx context.Context, id uuid.UUID, result []byte) error {
	task, err := r.GetTask(ctx, id)
	if err != nil {
		return err
	}
	if _, err := r.q.UpdateTaskCompleted(ctx, sqlc.UpdateTaskCompletedParams{
		ID:     id,
		Result: result,
	}); err != nil {
		return fmt.Errorf("store: complete task: %w", err)
	}

	// Check if all tasks for the job are finished
	tasks, err := r.ListTasks(ctx, task.JobID)
	logger := logpkg.From(ctx)
	if err == nil {
		allFinished := true
		anyFailed := false
		var lastErrMsg string
		for _, t := range tasks {
			logger.Info("store: CompleteTask check", zap.String("job_id", task.JobID.String()), zap.String("task_kind", string(t.Kind)), zap.String("task_status", string(t.Status)))
			if t.Status != model.TaskCompleted && t.Status != model.TaskFailed {
				allFinished = false
			}
			if t.Status == model.TaskFailed {
				anyFailed = true
				lastErrMsg = t.ErrorMsg
			}
		}
		logger.Info("store: CompleteTask summary", zap.String("job_id", task.JobID.String()), zap.Bool("all_finished", allFinished), zap.Bool("any_failed", anyFailed))
		if allFinished {
			status := model.JobCompleted
			errMsg := ""
			if anyFailed {
				status = model.JobFailed
				errMsg = "one or more tasks failed: " + lastErrMsg
			}
			job, err := r.UpdateStatus(ctx, task.JobID, status, "", errMsg, 0)
			if err != nil {
				logger.Error("store: failed to update job status", zap.Error(err))
			} else {
				logger.Info("store: updated job status successfully", zap.String("job_id", job.ID.String()), zap.String("status", string(job.Status)))
			}
		}
	} else {
		logger.Error("store: ListTasks failed", zap.Error(err))
	}
	return nil
}

func (r *Jobs) FailTask(ctx context.Context, id uuid.UUID, errMsg string) error {
	task, err := r.GetTask(ctx, id)
	if err != nil {
		return err
	}
	if _, err := r.q.UpdateTaskFailed(ctx, sqlc.UpdateTaskFailedParams{
		ID:       id,
		ErrorMsg: toPtrStr(errMsg),
	}); err != nil {
		return fmt.Errorf("store: fail task: %w", err)
	}

	// Check if all tasks for the job are finished
	tasks, err := r.ListTasks(ctx, task.JobID)
	logger := logpkg.From(ctx)
	if err == nil {
		allFinished := true
		anyFailed := false
		var lastErrMsg string
		for _, t := range tasks {
			logger.Info("store: FailTask check", zap.String("job_id", task.JobID.String()), zap.String("task_kind", string(t.Kind)), zap.String("task_status", string(t.Status)))
			if t.Status != model.TaskCompleted && t.Status != model.TaskFailed {
				allFinished = false
			}
			if t.Status == model.TaskFailed {
				anyFailed = true
				lastErrMsg = t.ErrorMsg
			}
		}
		logger.Info("store: FailTask summary", zap.String("job_id", task.JobID.String()), zap.Bool("all_finished", allFinished), zap.Bool("any_failed", anyFailed))
		if allFinished {
			status := model.JobCompleted
			errMsg := ""
			if anyFailed {
				status = model.JobFailed
				errMsg = "one or more tasks failed: " + lastErrMsg
			}
			job, err := r.UpdateStatus(ctx, task.JobID, status, "", errMsg, 0)
			if err != nil {
				logger.Error("store: failed to update job status", zap.Error(err))
			} else {
				logger.Info("store: updated job status successfully", zap.String("job_id", job.ID.String()), zap.String("status", string(job.Status)))
			}
		}
	} else {
		logger.Error("store: ListTasks failed", zap.Error(err))
	}
	return nil
}

func jobFromRow(j sqlc.Job) model.Job {
	return model.Job{
		ID:        j.ID,
		UploadID:  j.UploadID,
		Status:    model.JobStatus(j.Status),
		ErrorCode: fromPtrStr(j.ErrorCode),
		ErrorMsg:  fromPtrStr(j.ErrorMsg),
		Retries:   int(j.Retries),
		CreatedAt: j.CreatedAt,
		UpdatedAt: j.UpdatedAt,
	}
}

func taskFromRow(t sqlc.Task) model.Task {
	var started, finished *string
	if t.StartedAt != nil {
		s := t.StartedAt.Format("2006-01-02T15:04:05Z07:00")
		started = &s
	}
	if t.FinishedAt != nil {
		s := t.FinishedAt.Format("2006-01-02T15:04:05Z07:00")
		finished = &s
	}
	_ = started
	_ = finished
	return model.Task{
		ID:       t.ID,
		JobID:    t.JobID,
		Kind:     model.TaskKind(t.Kind),
		Status:   model.TaskStatus(t.Status),
		Attempt:  int(t.Attempt),
		Payload:  t.Payload,
		Result:   t.Result,
		ErrorMsg: fromPtrStr(t.ErrorMsg),
	}
}
