package client

import (
	"log/slog"
	"os"

	"claude-memory-sync/internal/domain"
)

const (
	envURL   = "PIPELINE_URL"
	envToken = "PIPELINE_TOKEN"
)

type Config struct {
	URL   string
	Token string
}

func ConfigFromEnv() (Config, error) {
	url := os.Getenv(envURL)
	if url == "" {
		return Config{}, domain.Error(domain.ErrValidation, "missing required env var", slog.String("var", envURL))
	}

	token := os.Getenv(envToken)
	if token == "" {
		return Config{}, domain.Error(domain.ErrValidation, "missing required env var", slog.String("var", envToken))
	}

	return Config{URL: url, Token: token}, nil
}
