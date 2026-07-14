package storage

import (
	"context"
	"time"
)

type Info struct {
	Key  string
	Size int64
	ETag string
}

type Storage interface {
	PresignPut(ctx context.Context, bucket, key string, size int64) (string, error)
	PresignGet(ctx context.Context, bucket, key string) (string, error)
	Stat(ctx context.Context, bucket, key string) (Info, error)
	Remove(ctx context.Context, bucket, key string) error
	RemovePrefix(ctx context.Context, bucket, prefix string) error
	Compose(ctx context.Context, bucket, dst string, sources []string) error
	EnsureBuckets(ctx context.Context, names ...string) error
	Close(ctx context.Context) error
}

type Config struct {
	Endpoint       string
	PublicEndpoint string
	Region         string
	AccessKey      string
	SecretKey      string
	BucketUploads  string
	BucketMedia    string
	PresignTTL     time.Duration
}
