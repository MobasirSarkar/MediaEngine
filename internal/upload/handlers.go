package upload

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	errs "github.com/MobasirSarkar/MediaEngine/internal/err"
	"github.com/MobasirSarkar/MediaEngine/internal/model"
	"github.com/MobasirSarkar/MediaEngine/internal/response"
)

type Handlers struct {
	svc *Service
}

func NewHandlers(svc *Service) *Handlers { return &Handlers{svc: svc} }

func (h *Handlers) Create(c *gin.Context) {
	var req CreateReq
	if !response.Bind(c, &req) {
		return
	}
	out, err := h.svc.Create(c.Request.Context(), req)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.JSON(c, 201, out)
}

func (h *Handlers) AppendChunk(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	n, err := strconv.Atoi(c.Param("n"))
	if err != nil {
		response.Fail(c, errs.Wrap(errs.ErrInvalid, errs.ErrInvalid, "chunk_no must be integer"))
		return
	}
	var req ChunkReq
	if !response.Bind(c, &req) {
		return
	}
	out, err := h.svc.AppendChunk(c.Request.Context(), id, n, req)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.JSON(c, 200, out)
}

func (h *Handlers) Complete(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req CompleteReq
	_ = c.ShouldBindJSON(&req)
	out, err := h.svc.Complete(c.Request.Context(), id, req)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.JSON(c, 200, out)
}

func (h *Handlers) Cancel(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	if err := h.svc.Cancel(c.Request.Context(), id); err != nil {
		response.Fail(c, err)
		return
	}
	response.JSON(c, 200, gin.H{"upload_id": id, "status": model.UploadCanceled})
}

func (h *Handlers) Resume(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	out, err := h.svc.Resume(c.Request.Context(), id)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.JSON(c, 200, out)
}

func parseID(c *gin.Context) (uuid.UUID, bool) {
	raw := c.Param("id")
	id, err := uuid.Parse(raw)
	if err != nil {
		response.Fail(c, errs.Wrap(errs.ErrInvalid, errs.ErrInvalid, "invalid upload id"))
		return uuid.Nil, false
	}
	return id, true
}
