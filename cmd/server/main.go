package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"time"

	"claude-memory-sync/internal/api"
	"claude-memory-sync/internal/store"
)

const (
	defaultStorageDir = "/data"
	defaultPort       = "8080"
	readHeaderTimeout = 5 * time.Second
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	ctx := context.Background()

	token := os.Getenv("MEMORY_TOKEN")
	if token == "" {
		slog.ErrorContext(ctx, "missing required env var", slog.String("var", "MEMORY_TOKEN"))
		os.Exit(1)
	}

	storageDir := os.Getenv("MEMORY_STORAGE_DIR")
	if storageDir == "" {
		storageDir = defaultStorageDir
	}

	port := os.Getenv("MEMORY_PORT")
	if port == "" {
		port = defaultPort
	}

	s, err := store.New(ctx, storageDir)
	if err != nil {
		slog.ErrorContext(ctx, "failed to initialize store", slog.String("error", err.Error()))
		os.Exit(1)
	}

	handler := api.New(s, token)
	addr := ":" + port

	slog.InfoContext(ctx, "starting memory-server", slog.String("addr", addr), slog.String("storage_dir", storageDir))

	srv := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: readHeaderTimeout,
	}
	if err := srv.ListenAndServe(); err != nil {
		slog.ErrorContext(ctx, "server stopped", slog.String("error", err.Error()))
		os.Exit(1)
	}
}
