package server

import (
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/MobasirSarkar/MediaEngine/internal/config"
	"github.com/MobasirSarkar/MediaEngine/internal/storage"
)

type Deps struct {
	Logger  *zap.Logger
	Upload  UploadHandlers
	Jobs    JobsHandlers
	Storage storage.Storage
	Config  *config.Config
}

type UploadHandlers interface {
	Create(c *gin.Context)
	AppendChunk(c *gin.Context)
	Complete(c *gin.Context)
	Cancel(c *gin.Context)
	Resume(c *gin.Context)
}

type JobsHandlers interface {
	Get(c *gin.Context)
	GetByUpload(c *gin.Context)
	Stream(c *gin.Context)
}

func Routes(r *gin.Engine, d Deps) {
	r.GET("/healthz", func(c *gin.Context) { c.JSON(200, gin.H{"status": "ok"}) })
	r.GET("/readyz", func(c *gin.Context) { c.JSON(200, gin.H{"ready": true}) })
	r.StaticFile("/", "./web/index.html")

	r.GET("/media/*filepath", func(c *gin.Context) {
		filepath := c.Param("filepath")
		filepath = strings.TrimPrefix(filepath, "/")
		if filepath == "" {
			c.JSON(400, gin.H{"error": "missing file path"})
			return
		}
		bucket := d.Config.S3.BucketMedia
		if strings.HasPrefix(filepath, "uploads/") {
			bucket = d.Config.S3.BucketUploads
		}
		urlStr, err := d.Storage.PresignGet(c.Request.Context(), bucket, filepath)
		if err != nil {
			c.JSON(500, gin.H{"error": fmt.Sprintf("presign: %v", err)})
			return
		}
		c.Redirect(307, urlStr)
	})

	uploads := r.Group("/uploads")
	{
		uploads.POST("", d.Upload.Create)
		uploads.POST("/:id/chunks/:n", d.Upload.AppendChunk)
		uploads.POST("/:id/complete", d.Upload.Complete)
		uploads.POST("/:id/cancel", d.Upload.Cancel)
		uploads.GET("/:id/resume", d.Upload.Resume)
	}

	jobs := r.Group("/jobs")
	{
		jobs.GET("/:id", d.Jobs.Get)
		jobs.GET("/upload/:uploadId", d.Jobs.GetByUpload)
		jobs.GET("/:id/events", d.Jobs.Stream)
	}
}
