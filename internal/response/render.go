package response

import (
	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"

	"github.com/MobasirSarkar/MediaEngine/internal/err"
)

const requestIDHeader = "X-Request-ID"

func RequestID(c *gin.Context) string {
	if v := c.GetHeader(requestIDHeader); v != "" {
		return v
	}
	if v, ok := c.Get("request_id"); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func JSON[T any](c *gin.Context, status int, data T) {
	c.JSON(status, Ok(data, RequestID(c)))
}

func Fail(c *gin.Context, err error) {
	c.JSON(errs.Status(err), FailBody(err, RequestID(c)))
}

func Bind(c *gin.Context, dst any) bool {
	if err := c.ShouldBindJSON(dst); err != nil {
		Fail(c, errs.Wrap(err, errs.ErrInvalid, "invalid request body"))
		return false
	}
	if err := defaultValidator.Struct(dst); err != nil {
		Fail(c, errs.Wrap(err, errs.ErrInvalid, "validation failed"))
		return false
	}
	return true
}

var defaultValidator = validator.New(validator.WithRequiredStructEnabled())
