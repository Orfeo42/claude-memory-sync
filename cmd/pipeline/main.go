package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"time"

	"claude-memory-sync/internal/domain"
	"claude-memory-sync/internal/pipeline"
	"claude-memory-sync/internal/pipeline/httpapi"
	"claude-memory-sync/internal/pipeline/postgres"
)

const readHeaderTimeout = 5 * time.Second

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	ctx := context.Background()
	if err := run(ctx); err != nil {
		domain.LogError(ctx, "pipeline-api stopped", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	db, err := postgres.Open(ctx, cfg.databaseURL)
	if err != nil {
		return domain.Error(err, "open pipeline database failed")
	}

	if err := postgres.Migrate(ctx, db); err != nil {
		return domain.Error(err, "migrate pipeline database failed")
	}
	slog.InfoContext(ctx, "pipeline database migrated")

	svc := pipeline.NewService(postgres.New(db))
	addr := ":" + cfg.port
	srv := &http.Server{
		Addr:              addr,
		Handler:           httpapi.New(ctx, svc, cfg.token, cfg.adminToken),
		ReadHeaderTimeout: readHeaderTimeout,
	}

	slog.InfoContext(ctx, "starting pipeline-api", slog.String("addr", addr))
	if err := srv.ListenAndServe(); err != nil {
		return domain.Error(err, "serve pipeline-api failed")
	}
	return nil
}
