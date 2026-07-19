package domain_test

import (
	"context"
	"errors"
	"log/slog"
	"runtime"
	"testing"

	"claude-memory-sync/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type recordingHandler struct {
	records []slog.Record
}

func (h *recordingHandler) Enabled(context.Context, slog.Level) bool {
	return true
}

func (h *recordingHandler) Handle(_ context.Context, record slog.Record) error {
	h.records = append(h.records, record)
	return nil
}

func (h *recordingHandler) WithAttrs([]slog.Attr) slog.Handler {
	return h
}

func (h *recordingHandler) WithGroup(string) slog.Handler {
	return h
}

func recordAttrs(t *testing.T, record slog.Record) map[string]slog.Value {
	t.Helper()

	attrs := make(map[string]slog.Value)
	record.Attrs(func(a slog.Attr) bool {
		attrs[a.Key] = a.Value
		return true
	})
	return attrs
}

func withRecordingHandler(t *testing.T) *recordingHandler {
	t.Helper()

	handler := &recordingHandler{}
	previous := slog.Default()
	slog.SetDefault(slog.New(handler))
	t.Cleanup(func() {
		slog.SetDefault(previous)
	})
	return handler
}

func TestLogError(t *testing.T) {
	t.Run("emits one error level record with merged attrs", func(t *testing.T) {
		handler := withRecordingHandler(t)
		cause := domain.WrapError(domain.ErrValidation, slog.String("field", "email"))

		domain.LogError(t.Context(), "validation failed", cause, slog.String("request_id", "abc"))

		require.Len(t, handler.records, 1)
		record := handler.records[0]
		assert.Equal(t, slog.LevelError, record.Level)
		assert.Equal(t, "validation failed", record.Message)

		attrs := recordAttrs(t, record)
		assert.Equal(t, "email", attrs["field"].String())
		assert.Equal(t, "abc", attrs["request_id"].String())
		require.Contains(t, attrs, "error")
	})

	t.Run("record source resolves to the caller, not log.go", func(t *testing.T) {
		handler := withRecordingHandler(t)

		domain.LogError(t.Context(), "boom", errors.New("boom"))

		require.Len(t, handler.records, 1)
		frames := runtime.CallersFrames([]uintptr{handler.records[0].PC})
		frame, _ := frames.Next()

		assert.Contains(t, frame.File, "log_test.go")
		assert.NotContains(t, frame.File, "domain/log.go")
	})
}

func TestLogWarn(t *testing.T) {
	t.Run("emits one warn level record with merged attrs", func(t *testing.T) {
		handler := withRecordingHandler(t)
		cause := domain.WrapError(domain.ErrUpstream, slog.String("api", "billing"))

		domain.LogWarn(t.Context(), "upstream degraded", cause)

		require.Len(t, handler.records, 1)
		record := handler.records[0]
		assert.Equal(t, slog.LevelWarn, record.Level)
		assert.Equal(t, "upstream degraded", record.Message)

		attrs := recordAttrs(t, record)
		assert.Equal(t, "billing", attrs["api"].String())
		require.Contains(t, attrs, "error")
	})

	t.Run("record source resolves to the caller, not log.go", func(t *testing.T) {
		handler := withRecordingHandler(t)

		domain.LogWarn(t.Context(), "boom", errors.New("boom"))

		require.Len(t, handler.records, 1)
		frames := runtime.CallersFrames([]uintptr{handler.records[0].PC})
		frame, _ := frames.Next()

		assert.Contains(t, frame.File, "log_test.go")
		assert.NotContains(t, frame.File, "domain/log.go")
	})
}
