package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"claude-memory-sync/internal/domain"
	"claude-memory-sync/internal/flows"
	"claude-memory-sync/internal/footprint"
	"claude-memory-sync/internal/llm"
	"claude-memory-sync/internal/pipeline/client"
	"claude-memory-sync/internal/synthapi"
	"claude-memory-sync/internal/synthsched"
	"claude-memory-sync/internal/synthsettings"
)

const (
	worker            = "synth"
	pipelineTimeout   = 30 * time.Second
	readHeaderTimeout = 5 * time.Second
	shutdownTimeout   = 30 * time.Second
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	os.Exit(serve())
}

func serve() int {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := run(ctx); err != nil {
		domain.LogError(ctx, "synth stopped", err)
		return 1
	}
	return 0
}

func run(ctx context.Context) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	pipe := client.New(client.Config{URL: cfg.pipeline, Token: cfg.token}, &http.Client{Timeout: pipelineTimeout})
	settings := synthsettings.New(pipe, cfg.oauthToken)
	runner := llm.New(settings)
	flowRunner := flows.New(
		flows.Config{ReposDir: cfg.reposDir, WorkDir: cfg.workDir, Lock: cfg.lock, Worker: worker},
		flows.Deps{LLM: runner, Recorder: recorder{store: pipe}, Settings: settings, Pipeline: pipe},
	)

	sched, err := synthsched.New(settings, flowRunner, pipe)
	if err != nil {
		return err
	}
	if err := sched.Start(ctx); err != nil {
		return err
	}

	srv := &http.Server{
		Addr: cfg.addr,
		Handler: synthapi.New(synthapi.Deps{
			Pipeline:  pipe,
			Settings:  settings,
			Flows:     flowRunner,
			Scheduler: sched,
			LLM:       runner,
			Footprint: footprint.Build,
			WorkDir:   cfg.workDir,
			WebDir:    cfg.webDir,
		}),
		ReadHeaderTimeout: readHeaderTimeout,
	}

	serveErr := make(chan error, 1)
	go func() {
		slog.InfoContext(ctx, "starting synth", slog.String("addr", cfg.addr))
		serveErr <- srv.ListenAndServe()
	}()

	select {
	case err := <-serveErr:
		return errors.Join(domain.Error(err, "serve synth failed"), sched.Shutdown())
	case <-ctx.Done():
	}

	slog.InfoContext(ctx, "shutting down synth")
	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return errors.Join(domain.Error(err, "shutdown synth server failed"), sched.Shutdown())
	}
	return sched.Shutdown()
}
