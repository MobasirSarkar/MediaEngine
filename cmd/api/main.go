package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/MobasirSarkar/MediaEngine/internal/config"
	"github.com/MobasirSarkar/MediaEngine/internal/db"
	sqlc "github.com/MobasirSarkar/MediaEngine/internal/db/sqlc"
	"github.com/MobasirSarkar/MediaEngine/internal/events"
	"github.com/MobasirSarkar/MediaEngine/internal/hub"
	"github.com/MobasirSarkar/MediaEngine/internal/jobs"
	logpkg "github.com/MobasirSarkar/MediaEngine/internal/log"
	"github.com/MobasirSarkar/MediaEngine/internal/server"
	"github.com/MobasirSarkar/MediaEngine/internal/storage"
	st "github.com/MobasirSarkar/MediaEngine/internal/store"
	"github.com/MobasirSarkar/MediaEngine/internal/upload"
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("startup: %v", err)
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

	rootCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	rootCtx = logpkg.With(rootCtx, logger)

	pool, err := db.New(rootCtx, cfg)
	if err != nil {
		return err
	}
	defer pool.Close()

	q := sqlc.New(pool)
	bus, err := events.New(rootCtx, events.Config{URL: cfg.NATS.URL, Stream: cfg.NATS.Stream})
	if err != nil {
		return fmt.Errorf("bus: %w", err)
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = bus.Close(shutdownCtx)
	}()
	if err := bus.EnsureStream(rootCtx); err != nil {
		logger.Warn("ensure stream", zap.Error(err))
	}

	objectStore, err := storage.NewS3(rootCtx, storage.Config{
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
	if err := objectStore.EnsureBuckets(rootCtx, cfg.S3.BucketUploads, cfg.S3.BucketMedia); err != nil {
		logger.Warn("ensure buckets", zap.Error(err))
	}

	h := hub.New(64)
	uploadSvc := upload.NewService(
		upload.Config{SessionTTL: cfg.Up.SessionTTL, Bucket: cfg.S3.BucketUploads},
		st.NewUploads(q), objectStore, bus,
	)
	jobSvc := jobs.NewService(st.NewJobs(q), bus)
	uploadH := upload.NewHandlers(uploadSvc)
	jobH := jobs.NewHandlers(jobSvc, h)

	orch := jobs.NewOrchestrator(jobSvc, bus, h)
	go func() {
		if err := orch.Run(rootCtx); err != nil {
			logger.Error("orchestrator", zap.Error(err))
		}
	}()

	engine := gin.New()
	engine.Use(
		server.RequestID(),
		server.Logger(logger),
		server.Recover(),
		server.Timeout(30*time.Second),
	)
	server.Routes(engine, server.Deps{
		Logger:  logger,
		Upload:  uploadH,
		Jobs:    jobH,
		Storage: objectStore,
		Config:  cfg,
	})

	srv := server.New(cfg, engine)
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Start() }()

	logger.Info("listening", zap.String("addr", srv.Addr()), zap.String("env", cfg.App.Env))
	select {
	case <-rootCtx.Done():
		logger.Info("shutting down")
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("server: %w", err)
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("shutdown", zap.Error(err))
	}
	stop()
	return nil
}
