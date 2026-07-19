package api

import (
	"errors"
	"log/slog"

	"github.com/danielgtaylor/huma/v2"

	"github.com/Orfeo42/claude-memory-sync/internal/store"
)

func mapStoreError(err error) error {
	switch {
	case errors.Is(err, store.ErrNotFound):
		return huma.Error404NotFound("file not found")
	case errors.Is(err, store.ErrInvalidPath), errors.Is(err, store.ErrInvalidNamespace):
		return huma.Error400BadRequest("invalid path")
	default:
		slog.Error("store operation failed", slog.String("error", err.Error()))
		return huma.Error500InternalServerError("internal error")
	}
}
