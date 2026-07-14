package response

import "github.com/MobasirSarkar/MediaEngine/internal/err"

type Envelope[T any] struct {
	Success   bool           `json:"success"`
	Data      T              `json:"data,omitempty"`
	Error     *Error         `json:"error,omitempty"`
	RequestID string         `json:"request_id,omitempty"`
	Meta      map[string]any `json:"meta,omitempty"`
}

type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func Ok[T any](data T, reqID string) Envelope[T] {
	return Envelope[T]{Success: true, Data: data, RequestID: reqID}
}

func errorBody(err error) *Error {
	if err == nil {
		return nil
	}
	var e *errs.Error
	if errs.As(err, &e) {
		return &Error{Code: e.Code, Message: e.Message}
	}
	if errs.Is(err, errs.ErrNotFound) {
		return &Error{Code: errs.ErrNotFound.Error(), Message: "not found"}
	}
	if errs.Is(err, errs.ErrConflict) {
		return &Error{Code: errs.ErrConflict.Error(), Message: "conflict"}
	}
	if errs.Is(err, errs.ErrInvalid) {
		return &Error{Code: errs.ErrInvalid.Error(), Message: "invalid"}
	}
	if errs.Is(err, errs.ErrExpired) {
		return &Error{Code: errs.ErrExpired.Error(), Message: "expired"}
	}
	return &Error{Code: errs.ErrInternal.Error(), Message: "internal error"}
}

func FailBody(err error, reqID string) Envelope[any] {
	return Envelope[any]{Success: false, RequestID: reqID, Error: errorBody(err)}
}
