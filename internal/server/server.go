package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/MobasirSarkar/MediaEngine/internal/config"
)

type Server struct {
	cfg    *config.Config
	engine *gin.Engine
	http   *http.Server
}

func New(cfg *config.Config, engine *gin.Engine) *Server {
	if !cfg.IsDev() {
		gin.SetMode(gin.ReleaseMode)
	}
	return &Server{cfg: cfg, engine: engine, http: &http.Server{
		Addr:              cfg.App.HTTPAddr,
		Handler:           engine,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      0, // uploads may be long; chunk presign handles their own timeout
		IdleTimeout:       2 * time.Minute,
	}}
}

func (s *Server) Start() error {
	if err := s.http.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("server: listen: %w", err)
	}
	return nil
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.http.Shutdown(ctx)
}

func (s *Server) Addr() string { return s.cfg.App.HTTPAddr }
