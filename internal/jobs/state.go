package jobs

import (
	"github.com/MobasirSarkar/MediaEngine/internal/model"
)

type transition struct {
	from model.JobStatus
	to   model.JobStatus
}

var allowed = map[transition]bool{
	{model.JobCreated, model.JobQueued}:    true,
	{model.JobQueued, model.JobRunning}:    true,
	{model.JobRunning, model.JobCompleted}: true,
	{model.JobRunning, model.JobFailed}:    true,
	{model.JobFailed, model.JobQueued}:     true,
}

func CanTransition(from, to model.JobStatus) bool {
	return allowed[transition{from, to}]
}
