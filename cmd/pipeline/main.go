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
	"claude-memory-sync/internal/runs"
	runshttpapi "claude-memory-sync/internal/runs/httpapi"
	runspostgres "claude-memory-sync/internal/runs/postgres"
	"claude-memory-sync/internal/settings"
	settingshttpapi "claude-memory-sync/internal/settings/httpapi"
	settingspostgres "claude-memory-sync/internal/settings/postgres"
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
	if err := settingspostgres.Migrate(ctx, db); err != nil {
		return domain.Error(err, "migrate settings tables failed")
	}
	if err := runspostgres.Migrate(ctx, db); err != nil {
		return domain.Error(err, "migrate runs tables failed")
	}
	slog.InfoContext(ctx, "pipeline database migrated")

	svc := pipeline.NewService(postgres.New(db))
	settingsSvc := settings.NewService(settingspostgres.New(db))
	runsSvc := runs.NewService(runspostgres.New(db))
	addr := ":" + cfg.port
	srv := &http.Server{
		Addr: addr,
		Handler: httpapi.New(ctx, svc, cfg.token, cfg.adminToken,
			settingshttpapi.Register(settingsSvc),
			runshttpapi.Register(runsSvc),
		),
		ReadHeaderTimeout: readHeaderTimeout,
	}

	slog.InfoContext(ctx, "starting pipeline-api", slog.String("addr", addr))
	if err := srv.ListenAndServe(); err != nil {
		return domain.Error(err, "serve pipeline-api failed")
	}
	return nil
}
