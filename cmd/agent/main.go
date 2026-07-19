package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	"claude-memory-sync/internal/syncer"
)

const (
	defaultClaudeDir = "/claude"
	defaultStateDir  = "/state"
	defaultInterval  = 15 * time.Minute
)

func requireEnv(name string) string {
	value := os.Getenv(name)
	if value == "" {
		slog.Error("missing required env var", slog.String("var", name))
		os.Exit(1)
	}
	return value
}

func loadConfig() syncer.Config {
	cfg := syncer.Config{
		ServerURL:  requireEnv("MEMORY_SERVER_URL"),
		Token:      requireEnv("MEMORY_TOKEN"),
		ClientID:   requireEnv("MEMORY_CLIENT_ID"),
		SlugPrefix: requireEnv("MEMORY_SLUG_PREFIX"),
		ClaudeDir:  defaultClaudeDir,
		StateDir:   defaultStateDir,
		Interval:   defaultInterval,
	}

	if v := os.Getenv("MEMORY_CLAUDE_DIR"); v != "" {
		cfg.ClaudeDir = v
	}
	if v := os.Getenv("MEMORY_STATE_DIR"); v != "" {
		cfg.StateDir = v
	}
	if v := os.Getenv("MEMORY_INTERVAL"); v != "" {
		interval, err := time.ParseDuration(v)
		if err != nil {
			slog.Error("invalid MEMORY_INTERVAL", slog.String("value", v), slog.String("error", err.Error()))
			os.Exit(1)
		}
		cfg.Interval = interval
	}

	return cfg
}

func runCycle(agent *syncer.Agent) {
	ctx := context.Background()
	if err := agent.RunCycle(ctx); err != nil {
		slog.Error("sync cycle failed", slog.String("error", err.Error()))
		return
	}
	slog.Info("sync cycle completed")
}

func runOnceEnabled() bool {
	return os.Getenv("MEMORY_RUN_ONCE") == "true"
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	cfg := loadConfig()
	client := syncer.NewHTTPClient(cfg.ServerURL, cfg.Token, cfg.ClientID)
	agent := syncer.New(cfg, client)

	slog.Info("starting memory-agent",
		slog.String("serverUrl", cfg.ServerURL),
		slog.String("clientId", cfg.ClientID),
		slog.String("slugPrefix", cfg.SlugPrefix),
		slog.Duration("interval", cfg.Interval),
	)

	runCycle(agent)

	if runOnceEnabled() {
		return
	}

	ticker := time.NewTicker(cfg.Interval)
	defer ticker.Stop()

	for range ticker.C {
		runCycle(agent)
	}
}
