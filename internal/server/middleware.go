package server

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	logpkg "github.com/MobasirSarkar/MediaEngine/internal/log"
)

const (
	headerRequestID = "X-Request-ID"
	ctxKeyReqID     = "request_id"
)

// RequestID assigns or echoes a request id and stores it in the context.
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader(headerRequestID)
		if id == "" {
			id = uuid.NewString()
		}
		c.Set(ctxKeyReqID, id)
		c.Writer.Header().Set(headerRequestID, id)
		c.Next()
	}
}

// Logger attaches a request-scoped logger to ctx and logs once per request.
func Logger(base *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		l := base.With(
			zap.String("request_id", c.GetString(ctxKeyReqID)),
			zap.String("method", c.Request.Method),
			zap.String("path", c.FullPath()),
		)
		ctx := logpkg.With(c.Request.Context(), l)
		c.Request = c.Request.WithContext(ctx)
		start := time.Now()
		c.Next()
		l.Info("request",
			zap.Int("status", c.Writer.Status()),
			zap.Duration("dur", time.Since(start)),
			zap.Int("size", c.Writer.Size()),
		)
	}
}

// Recover converts panics to internal errors without leaking stack to client.
func Recover() gin.HandlerFunc {
	return gin.CustomRecovery(func(c *gin.Context, _ any) {
		l := logpkg.From(c.Request.Context())
		l.Error("panic", zap.Any("err", c.Errors.Last()))
		c.AbortWithStatusJSON(500, gin.H{"success": false, "error": gin.H{"code": "internal", "message": "internal error"}})
	})
}

// Timeout aborts requests that exceed the configured duration.
func Timeout(d time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), d) //nolint:gosimple // shadowed name intentional
		defer cancel()
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}
