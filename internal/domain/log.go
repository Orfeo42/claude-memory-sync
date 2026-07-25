package domain

import (
	"context"
	"log/slog"
	"runtime"
	"time"
)

func LogError(ctx context.Context, msg string, err error, attrs ...slog.Attr) {
	logAt(ctx, slog.LevelError, msg, err, attrs)
}

func LogWarn(ctx context.Context, msg string, err error, attrs ...slog.Attr) {
	logAt(ctx, slog.LevelWarn, msg, err, attrs)
}

func logAt(ctx context.Context, level slog.Level, msg string, err error, attrs []slog.Attr) {
	handler := slog.Default().Handler()
	if !handler.Enabled(ctx, level) {
		return
	}
	var pcs [1]uintptr
	runtime.Callers(3, pcs[:])
	record := slog.NewRecord(time.Now(), level, msg, pcs[0])
	record.AddAttrs(ErrorAttrs(err)...)
	record.AddAttrs(attrs...)
	record.AddAttrs(slog.Any("error", err))
	_ = handler.Handle(ctx, record)
}
