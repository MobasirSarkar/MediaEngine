package log

import (
	"context"
	"fmt"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type ctxKey struct{}

func New(level, format, env string) (*zap.Logger, error) {
	lvl, err := zapcore.ParseLevel(level)
	if err != nil {
		return nil, fmt.Errorf("log: parse level %q: %w", level, err)
	}
	var enc zapcore.EncoderConfig
	if env == "dev" {
		enc = zap.NewDevelopmentEncoderConfig()
	} else {
		enc = zap.NewProductionEncoderConfig()
	}
	var cfg zap.Config
	cfg.Level = zap.NewAtomicLevelAt(lvl)
	cfg.EncoderConfig = enc
	cfg.OutputPaths = []string{"stdout"}
	cfg.ErrorOutputPaths = []string{"stderr"}
	if format == "json" || env != "dev" {
		cfg.Encoding = "json"
	} else {
		cfg.Encoding = "console"
	}
	l, err := cfg.Build()
	if err != nil {
		return nil, fmt.Errorf("log: build: %w", err)
	}
	return l, nil
}

func With(ctx context.Context, l *zap.Logger) context.Context {
	return context.WithValue(ctx, ctxKey{}, l)
}

func From(ctx context.Context) *zap.Logger {
	if v, ok := ctx.Value(ctxKey{}).(*zap.Logger); ok && v != nil {
		return v
	}
	return zap.NewNop()
}
