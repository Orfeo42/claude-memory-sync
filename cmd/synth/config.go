package main

import (
	"log/slog"
	"os"

	"claude-memory-sync/internal/domain"
)

type config struct {
	addr       string
	webDir     string
	pipeline   string
	token      string
	reposDir   string
	workDir    string
	lock       string
	oauthToken string
}

func loadConfig() (config, error) {
	cfg := config{
		addr:       envOr("SYNTH_ADDR", ":8082"),
		webDir:     envOr("SYNTH_WEB_DIR", "/srv/web"),
		pipeline:   os.Getenv("PIPELINE_URL"),
		token:      os.Getenv("PIPELINE_TOKEN"),
		reposDir:   envOr("MEMORY_REPOS_DIR", "/data/repos"),
		workDir:    envOr("MEMORY_WORK_DIR", "/data/work/canonical"),
		lock:       envOr("MEMORY_LOCK", "/data/state/canonical.lock"),
		oauthToken: os.Getenv("CLAUDE_CODE_OAUTH_TOKEN"),
	}
	if cfg.pipeline == "" {
		return config{}, domain.Error(domain.ErrValidation, "missing required env var", slog.String("var", "PIPELINE_URL"))
	}
	if cfg.token == "" {
		return config{}, domain.Error(domain.ErrValidation, "missing required env var", slog.String("var", "PIPELINE_TOKEN"))
	}
	return cfg, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
