package errs

import "net/http"

var statusByCode = map[string]int{
	ErrNotFound.Error():     http.StatusNotFound,
	ErrConflict.Error():     http.StatusConflict,
	ErrInvalid.Error():      http.StatusBadRequest,
	ErrExpired.Error():      http.StatusGone,
	ErrUnauthorized.Error(): http.StatusUnauthorized,
	ErrInternal.Error():     http.StatusInternalServerError,
}

func Status(err error) int {
	if err == nil {
		return http.StatusOK
	}
	var e *Error
	if As(err, &e) {
		if s, ok := statusByCode[e.Code]; ok {
			return s
		}
	}
	if Is(err, ErrNotFound) {
		return http.StatusNotFound
	}
	if Is(err, ErrConflict) {
		return http.StatusConflict
	}
	if Is(err, ErrInvalid) {
		return http.StatusBadRequest
	}
	if Is(err, ErrExpired) {
		return http.StatusGone
	}
	if Is(err, ErrUnauthorized) {
		return http.StatusUnauthorized
	}
	return http.StatusInternalServerError
}
