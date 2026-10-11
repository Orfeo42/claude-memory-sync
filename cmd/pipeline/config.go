package main

import (
	"log/slog"
	"os"

	"claude-memory-sync/internal/domain"
)

const defaultPort = "8090"

type config struct {
	databaseURL string
	token       string
	adminToken  string
	port        string
}

func loadConfig() (config, error) {
	cfg := config{
		databaseURL: os.Getenv("PIPELINE_DATABASE_URL"),
		token:       os.Getenv("PIPELINE_TOKEN"),
		adminToken:  os.Getenv("PIPELINE_ADMIN_TOKEN"),
		port:        os.Getenv("PIPELINE_PORT"),
	}

	if cfg.databaseURL == "" {
		return config{}, domain.Error(domain.ErrValidation, "missing required env var", slog.String("var", "PIPELINE_DATABASE_URL"))
	}
	if cfg.token == "" {
		return config{}, domain.Error(domain.ErrValidation, "missing required env var", slog.String("var", "PIPELINE_TOKEN"))
	}
	if cfg.adminToken == "" {
		return config{}, domain.Error(domain.ErrValidation, "missing required env var", slog.String("var", "PIPELINE_ADMIN_TOKEN"))
	}
	if cfg.adminToken == cfg.token {
		return config{}, domain.Error(domain.ErrValidation, "PIPELINE_ADMIN_TOKEN must differ from PIPELINE_TOKEN")
	}
	if cfg.port == "" {
		cfg.port = defaultPort
	}

	return cfg, nil
}
