package domain

import (
	"fmt"
	"log/slog"
	"net/http"
)

type UpstreamError struct {
	API        string
	StatusCode int
	Body       string
}

func (e *UpstreamError) sentinel() sentinelError {
	switch {
	case e.StatusCode == http.StatusNotFound:
		return ErrNotFound
	case e.StatusCode == http.StatusConflict:
		return ErrConflict
	case e.StatusCode == http.StatusUnauthorized || e.StatusCode == http.StatusForbidden:
		return ErrInternal
	case e.StatusCode >= http.StatusInternalServerError:
		return ErrUpstream
	case e.StatusCode >= http.StatusBadRequest:
		return ErrValidation
	default:
		return ErrInternal
	}
}

func (e *UpstreamError) Error() string {
	return fmt.Sprintf("upstream %s returned status %d: %s", e.API, e.StatusCode, e.Body)
}

func (e *UpstreamError) Is(target error) bool {
	return target == error(e.sentinel())
}

func (e *UpstreamError) IsOurFault() bool {
	return e.sentinel() == ErrInternal
}

func (e *UpstreamError) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("api", e.API),
		slog.Int("status_code", e.StatusCode),
		slog.String("response_body", e.Body),
	)
}

type UpstreamUnreachableError struct {
	API   string
	Cause error
}

func (e *UpstreamUnreachableError) Error() string {
	return fmt.Sprintf("upstream %s is unreachable: %s", e.API, e.Cause)
}

func (e *UpstreamUnreachableError) Unwrap() error {
	return e.Cause
}

func (e *UpstreamUnreachableError) Is(target error) bool {
	return target == error(ErrUpstream)
}

func (e *UpstreamUnreachableError) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("api", e.API),
		slog.String("cause", e.Cause.Error()),
	)
}
