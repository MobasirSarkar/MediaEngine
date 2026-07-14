package main

import (
	"context"
	"fmt"
	"log"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"

	"github.com/MobasirSarkar/MediaEngine/internal/config"
	"github.com/MobasirSarkar/MediaEngine/internal/db"
	sqlc "github.com/MobasirSarkar/MediaEngine/internal/db/sqlc"
	"github.com/MobasirSarkar/MediaEngine/internal/events"
	logpkg "github.com/MobasirSarkar/MediaEngine/internal/log"
	"github.com/MobasirSarkar/MediaEngine/internal/storage"
	st "github.com/MobasirSarkar/MediaEngine/internal/store"
	"github.com/MobasirSarkar/MediaEngine/internal/workers/cleanup"
	"github.com/MobasirSarkar/MediaEngine/internal/workers/compress"
	"github.com/MobasirSarkar/MediaEngine/internal/workers/metadata"
	"github.com/MobasirSarkar/MediaEngine/internal/workers/thumbnail"
	"github.com/MobasirSarkar/MediaEngine/internal/workers/transcode"
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("worker: %v", err)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	logger, err := logpkg.New(cfg.Log.Level, cfg.Log.Format, cfg.App.Env)
	if err != nil {
		return fmt.Errorf("logger: %w", err)
	}
	defer func() { _ = logger.Sync() }()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	ctx = logpkg.With(ctx, logger)

	pool, err := db.New(ctx, cfg)
	if err != nil {
		return err
	}
	defer pool.Close()
	q := sqlc.New(pool)

	bus, err := events.New(ctx, events.Config{URL: cfg.NATS.URL, Stream: cfg.NATS.Stream})
	if err != nil {
		return fmt.Errorf("bus: %w", err)
	}
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = bus.Close(closeCtx)
	}()
	if err := bus.EnsureStream(ctx); err != nil {
		logger.Warn("ensure stream", zap.Error(err))
	}

	objectStore, err := storage.NewS3(ctx, storage.Config{
		Endpoint:       cfg.S3.Endpoint,
		PublicEndpoint: cfg.S3.PublicEndpoint,
		Region:         cfg.S3.Region,
		AccessKey:      cfg.S3.AccessKey,
		SecretKey:      cfg.S3.SecretKey,
		BucketUploads:  cfg.S3.BucketUploads,
		BucketMedia:    cfg.S3.BucketMedia,
		PresignTTL:     cfg.S3.PresignTTL,
	})
	if err != nil {
		return fmt.Errorf("storage: %w", err)
	}

	repo := st.NewJobs(q)

	runners := []func() error{
		func() error {
			return metadata.New(metadata.Config{Bucket: cfg.S3.BucketUploads}, repo, bus, objectStore).Run(ctx)
		},
		func() error {
			return thumbnail.Run(ctx, repo, bus, objectStore, cfg.S3.BucketUploads, cfg.S3.BucketMedia)
		},
		func() error {
			return transcode.Run(ctx, repo, bus, objectStore, cfg.S3.BucketUploads, cfg.S3.BucketMedia)
		},
		func() error {
			return compress.Run(ctx, repo, bus)
		},
		func() error {
			return cleanup.Run(ctx, bus, objectStore, cfg.S3.BucketUploads)
		},
	}
	for _, start := range runners {
		go func(start func() error) {
			if err := start(); err != nil {
				logger.Error("worker", zap.Error(err))
			}
		}(start)
	}
	logger.Info("worker started, listening for tasks...")
	<-ctx.Done()
	logger.Info("worker shutdown")
	stop()
	return nil
}
