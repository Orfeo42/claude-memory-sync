package main

import (
	"log/slog"
	"net/http"
	"os"

	"claude-memory-sync/internal/api"
	"claude-memory-sync/internal/store"
)

const (
	defaultStorageDir = "/data"
	defaultPort       = "8080"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	token := os.Getenv("MEMORY_TOKEN")
	if token == "" {
		slog.Error("missing required env var", slog.String("var", "MEMORY_TOKEN"))
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

	s, err := store.New(storageDir)
	if err != nil {
		slog.Error("failed to initialize store", slog.String("error", err.Error()))
		os.Exit(1)
	}

	handler := api.New(s, token)
	addr := ":" + port

	slog.Info("starting memory-server", slog.String("addr", addr), slog.String("storageDir", storageDir))
	if err := http.ListenAndServe(addr, handler); err != nil {
		slog.Error("server stopped", slog.String("error", err.Error()))
		os.Exit(1)
	}
}
