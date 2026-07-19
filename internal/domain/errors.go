package domain

import (
	"errors"
	"log/slog"
	"net/http"
	"path/filepath"
	"runtime"
	"strconv"
)

type sentinelError string

func (e sentinelError) Error() string {
	return string(e)
}

const (
	ErrNotFound     sentinelError = "not found"
	ErrUnauthorized sentinelError = "unauthorized"
	ErrValidation   sentinelError = "validation failed"
	ErrConflict     sentinelError = "conflict"
	ErrUpstream     sentinelError = "upstream service unavailable"
	ErrInternal     sentinelError = "internal server error"
)

type contextError struct {
	err   error
	msg   string
	attrs []slog.Attr
}

func (e *contextError) Error() string {
	if e.msg == "" {
		return e.err.Error()
	}
	return e.msg + ": " + e.err.Error()
}

func (e *contextError) Unwrap() error {
	return e.err
}

func originAttr(skip int) slog.Attr {
	_, file, line, ok := runtime.Caller(skip)
	if !ok {
		return slog.String("origin", "unknown")
	}
	return slog.String("origin", filepath.Base(file)+":"+strconv.Itoa(line))
}

func Error(err error, msg string, attrs ...slog.Attr) error {
	if err == nil {
		return nil
	}
	return &contextError{
		err:   err,
		msg:   msg,
		attrs: append(attrs, originAttr(2)),
	}
}

func WrapError(err error, attrs ...slog.Attr) error {
	if err == nil {
		return nil
	}
	return &contextError{
		err:   err,
		attrs: append(attrs, originAttr(2)),
	}
}

func ErrorAttrs(err error) []slog.Attr {
	var attrs []slog.Attr
	var origins []string
	cur := err
	for cur != nil {
		var ce *contextError
		if !errors.As(cur, &ce) {
			break
		}
		for _, a := range ce.attrs {
			if a.Key == "origin" {
				origins = append(origins, a.Value.String())
				continue
			}
			attrs = append(attrs, a)
		}
		cur = errors.Unwrap(ce)
	}
	if len(origins) == 0 {
		return attrs
	}
	reversed := make([]string, len(origins))
	for i, o := range origins {
		reversed[len(origins)-1-i] = o
	}
	return append(attrs, slog.Any("origin", reversed))
}

func StatusCodeFromError(err error) int {
	switch {
	case errors.Is(err, ErrNotFound):
		return http.StatusNotFound
	case errors.Is(err, ErrUnauthorized):
		return http.StatusUnauthorized
	case errors.Is(err, ErrConflict):
		return http.StatusConflict
	case errors.Is(err, ErrUpstream):
		return http.StatusBadGateway
	case errors.Is(err, ErrValidation):
		return http.StatusBadRequest
	default:
		return http.StatusInternalServerError
	}
}

func firstMessage(err error) (string, bool) {
	cur := err
	for cur != nil {
		var ce *contextError
		if !errors.As(cur, &ce) {
			return "", false
		}
		if ce.msg != "" {
			return ce.msg, true
		}
		cur = errors.Unwrap(ce)
	}
	return "", false
}

func genericMessage(statusCode int) string {
	switch statusCode {
	case http.StatusNotFound:
		return "the requested resource was not found"
	case http.StatusUnauthorized:
		return "authentication is required"
	case http.StatusConflict:
		return "the request could not be completed due to a conflict"
	case http.StatusBadGateway:
		return "an upstream service is unavailable"
	case http.StatusBadRequest:
		return "the request was invalid"
	default:
		return "an internal error occurred"
	}
}

func UserMessage(err error) string {
	if msg, ok := firstMessage(err); ok {
		return msg
	}
	return genericMessage(StatusCodeFromError(err))
}

func UserMessageOr(err error, fallback string) string {
	if msg, ok := firstMessage(err); ok {
		return msg
	}
	return fallback
}
