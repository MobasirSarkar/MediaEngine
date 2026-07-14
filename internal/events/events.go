package events

import "context"

type Subject = string

const (
	UploadCompleted Subject = "pipeline.upload.completed"
	UploadCanceled  Subject = "pipeline.upload.canceled"
	JobCreated      Subject = "pipeline.job.created"
	TaskCreated     Subject = "pipeline.task.created"
)

type Handler func(ctx context.Context, msg Message) error

type Message struct {
	Subject string
	Payload []byte
	Headers map[string]string
	Ack     func() error
	Nak     func() error
}

type Events interface {
	Publish(ctx context.Context, subject Subject, payload []byte, headers map[string]string) error
	Subscribe(ctx context.Context, subject Subject, durable string, handler Handler) error
	Close(ctx context.Context) error
	EnsureStream(ctx context.Context) error
}
