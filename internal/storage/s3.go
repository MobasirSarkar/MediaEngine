package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	errs "github.com/MobasirSarkar/MediaEngine/internal/err"
)

type s3Storage struct {
	client     *minio.Client
	signClient *minio.Client
	presignTTL time.Duration
	uploadsBkt string
	mediaBkt   string
}

func NewS3(ctx context.Context, cfg Config) (Storage, error) {
	if cfg.Endpoint == "" {
		return nil, errs.New(errs.ErrInvalid, "s3 endpoint required", nil)
	}
	endpoint := stripScheme(cfg.Endpoint)
	secure := strings.HasPrefix(strings.ToLower(cfg.Endpoint), "https://")
	cli, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: secure,
		Region: cfg.Region,
	})
	if err != nil {
		return nil, fmt.Errorf("storage: new client: %w", err)
	}

	// Create signClient (defaults to endpoint if no PublicEndpoint is provided)
	signEndpoint := cfg.Endpoint
	if cfg.PublicEndpoint != "" {
		signEndpoint = cfg.PublicEndpoint
	}
	signSecure := strings.HasPrefix(strings.ToLower(signEndpoint), "https://")
	signCli, err := minio.New(stripScheme(signEndpoint), &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: signSecure,
		Region: cfg.Region,
	})
	if err != nil {
		return nil, fmt.Errorf("storage: new sign client: %w", err)
	}

	ttl := cfg.PresignTTL
	if ttl <= 0 {
		ttl = 15 * time.Minute
	}
	return &s3Storage{
		client:     cli,
		signClient: signCli,
		presignTTL: ttl,
		uploadsBkt: cfg.BucketUploads,
		mediaBkt:   cfg.BucketMedia,
	}, nil
}

func (s *s3Storage) Close(_ context.Context) error { return nil }

func (s *s3Storage) PresignPut(ctx context.Context, bucket, key string, _ int64) (string, error) {
	urlObj, err := s.signClient.PresignedPutObject(ctx, bucket, key, s.presignTTL)
	if err != nil {
		return "", fmt.Errorf("storage: presign put: %w", err)
	}
	return urlObj.String(), nil
}

func (s *s3Storage) PresignGet(ctx context.Context, bucket, key string) (string, error) {
	urlObj, err := s.signClient.PresignedGetObject(ctx, bucket, key, s.presignTTL, nil)
	if err != nil {
		return "", fmt.Errorf("storage: presign get: %w", err)
	}
	return urlObj.String(), nil
}

func (s *s3Storage) Stat(ctx context.Context, bucket, key string) (Info, error) {
	st, err := s.client.StatObject(ctx, bucket, key, minio.StatObjectOptions{})
	if err != nil {
		errResp := minio.ToErrorResponse(err)
		if errResp.Code == "NoSuchKey" {
			return Info{}, errs.ErrNotFound
		}
		return Info{}, fmt.Errorf("storage: stat: %w", err)
	}
	return Info{Key: key, Size: st.Size, ETag: st.ETag}, nil
}

func (s *s3Storage) Remove(ctx context.Context, bucket, key string) error {
	if err := s.client.RemoveObject(ctx, bucket, key, minio.RemoveObjectOptions{}); err != nil {
		return fmt.Errorf("storage: remove: %w", err)
	}
	return nil
}

func (s *s3Storage) Compose(ctx context.Context, bucket, dst string, sources []string) error {
	if len(sources) == 0 {
		return errs.New(errs.ErrInvalid, "compose: empty sources", nil)
	}
	srcs := make([]minio.CopySrcOptions, 0, len(sources))
	for _, k := range sources {
		srcs = append(srcs, minio.CopySrcOptions{Bucket: bucket, Object: k})
	}
	dstOpts := minio.CopyDestOptions{Bucket: bucket, Object: dst}
	if _, err := s.client.ComposeObject(ctx, dstOpts, srcs...); err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		return fmt.Errorf("storage: compose: %w", err)
	}
	return nil
}

func (s *s3Storage) EnsureBuckets(ctx context.Context, names ...string) error {
	for _, name := range names {
		if name == "" {
			continue
		}
		exists, err := s.client.BucketExists(ctx, name)
		if err != nil {
			return fmt.Errorf("storage: bucket exists %s: %w", name, err)
		}
		if exists {
			continue
		}
		if err := s.client.MakeBucket(ctx, name, minio.MakeBucketOptions{}); err != nil {
			return fmt.Errorf("storage: make bucket %s: %w", name, err)
		}
	}
	return nil
}

func (s *s3Storage) RemovePrefix(ctx context.Context, bucket, prefix string) error {
	objectsCh := make(chan minio.ObjectInfo)

	go func() {
		defer close(objectsCh)
		opts := minio.ListObjectsOptions{
			Prefix:    prefix,
			Recursive: true,
		}
		for object := range s.client.ListObjects(ctx, bucket, opts) {
			if object.Err != nil {
				return
			}
			objectsCh <- object
		}
	}()

	errorCh := s.client.RemoveObjects(ctx, bucket, objectsCh, minio.RemoveObjectsOptions{})
	for e := range errorCh {
		if e.Err != nil {
			return fmt.Errorf("storage: remove prefix %s: %w", prefix, e.Err)
		}
	}
	return nil
}
