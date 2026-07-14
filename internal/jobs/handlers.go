package jobs

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	errs "github.com/MobasirSarkar/MediaEngine/internal/err"
	"github.com/MobasirSarkar/MediaEngine/internal/hub"
	"github.com/MobasirSarkar/MediaEngine/internal/response"
)

type Handlers struct {
	svc *Service
	hub *hub.Hub
}

func NewHandlers(svc *Service, h *hub.Hub) *Handlers {
	return &Handlers{svc: svc, hub: h}
}

func (h *Handlers) Get(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Fail(c, errs.Wrap(errs.ErrInvalid, errs.ErrInvalid, "invalid job id"))
		return
	}
	job, err := h.svc.Get(c.Request.Context(), id)
	if err != nil {
		response.Fail(c, err)
		return
	}
	tasks, err := h.svc.ListTasks(c.Request.Context(), id)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.JSON(c, 200, gin.H{"job": job, "tasks": tasks})
}

func (h *Handlers) GetByUpload(c *gin.Context) {
	id, err := uuid.Parse(c.Param("uploadId"))
	if err != nil {
		response.Fail(c, errs.Wrap(errs.ErrInvalid, errs.ErrInvalid, "invalid upload id"))
		return
	}
	job, err := h.svc.GetByUpload(c.Request.Context(), id)
	if err != nil {
		response.Fail(c, err)
		return
	}
	tasks, err := h.svc.ListTasks(c.Request.Context(), job.ID)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.JSON(c, 200, gin.H{"job": job, "tasks": tasks})
}

func (h *Handlers) Stream(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Fail(c, errs.Wrap(errs.ErrInvalid, errs.ErrInvalid, "invalid job id"))
		return
	}
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	key := id.String()
	ch, cancel := h.hub.Subscribe(key)
	defer cancel()
	c.Stream(func(w io.Writer) bool {
		select {
		case <-c.Request.Context().Done():
			return false
		case e, ok := <-ch:
			if !ok {
				return false
			}
			data, _ := json.Marshal(e.Data)
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", e.Type, data)
			return true
		}
	})
}
