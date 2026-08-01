package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"claude-memory-sync/internal/api"
	"claude-memory-sync/internal/githook"
	"claude-memory-sync/internal/gitserver"
)

const (
	defaultStorageDir = "/data"
	defaultPort       = "8080"
	readHeaderTimeout = 5 * time.Second
	reposDirName      = "repos"
	githookFlag       = "-githook-pre-receive"
	gitDirEnvVar      = "GIT_DIR"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == githookFlag {
		runPreReceiveHook()
		return
	}

	runServer()
}

func runPreReceiveHook() {
	gitDir := os.Getenv(gitDirEnvVar)
	if gitDir == "" {
		fmt.Fprintln(os.Stderr, "githook: missing required env var GIT_DIR")
		os.Exit(1)
	}

	if err := githook.Validate(context.Background(), gitDir, os.Stdin); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}

	os.Exit(0)
}

func runServer() {
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

	reposRoot := filepath.Join(storageDir, reposDirName)
	if err := gitserver.EnsureCanonical(ctx, reposRoot); err != nil {
		slog.ErrorContext(ctx, "failed to initialize canonical repository", slog.String("error", err.Error()))
		os.Exit(1)
	}

	gitHandler := gitserver.New(reposRoot)
	handler := api.New(token, gitHandler)
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
