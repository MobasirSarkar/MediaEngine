package errs

import (
	"errors"
	"fmt"
)

type Error struct {
	Code    string
	Message string
	Cause   error
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.Cause != nil {
		return fmt.Sprintf("%s: %s: %s", e.Code, e.Message, e.Cause.Error())
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *Error) Unwrap() error { return e.Cause }

var (
	ErrNotFound     = errors.New("not_found")
	ErrConflict     = errors.New("conflict")
	ErrInvalid      = errors.New("invalid_input")
	ErrExpired      = errors.New("expired")
	ErrUnauthorized = errors.New("unauthorized")
	ErrInternal     = errors.New("internal")
)

func New(code error, msg string, cause error) *Error {
	if code == nil {
		code = ErrInternal
	}
	return &Error{Code: code.Error(), Message: msg, Cause: cause}
}

func Wrap(cause error, code error, msg string) *Error {
	if cause == nil {
		return nil
	}
	return New(code, msg, cause)
}

func Is(err, target error) bool { return errors.Is(err, target) }

func As(err error, target **Error) bool { return errors.As(err, target) }
