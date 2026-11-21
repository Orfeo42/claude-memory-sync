package synthsched

import (
	"context"
	"errors"
	"log/slog"
	"sync"

	"github.com/go-co-op/gocron/v2"
	"github.com/google/uuid"

	"claude-memory-sync/internal/domain"
	"claude-memory-sync/internal/flows"
	"claude-memory-sync/internal/runs"
)

const (
	pruneName      = "prune-runs"
	pruneCron      = "30 2 * * *"
	pruneRetention = 90
)

type Settings interface {
	Schedule(ctx context.Context, flow flows.Name) (string, error)
}

type Runner interface {
	Run(ctx context.Context, flow flows.Name, trigger runs.Trigger) (string, error)
}

type Runs interface {
	InterruptRuns(ctx context.Context, flow string) (int, error)
	PruneRuns(ctx context.Context, olderThanDays int) (int, error)
}

//go:generate go tool moq -out synthsched_mock.go . Settings:MockSettings Runner:MockRunner Runs:MockRuns

type Scheduler struct {
	settings  Settings
	runner    Runner
	runs      Runs
	scheduler gocron.Scheduler

	mu   sync.Mutex
	jobs map[flows.Name]uuid.UUID
}

func New(settings Settings, runner Runner, runs Runs) (*Scheduler, error) {
	scheduler, err := gocron.NewScheduler()
	if err != nil {
		return nil, domain.Error(err, "create scheduler")
	}
	return &Scheduler{
		settings:  settings,
		runner:    runner,
		runs:      runs,
		scheduler: scheduler,
		jobs:      make(map[flows.Name]uuid.UUID),
	}, nil
}

func (s *Scheduler) Start(ctx context.Context) error {
	interrupted, err := s.runs.InterruptRuns(ctx, "")
	if err != nil {
		return domain.Error(err, "interrupt stale runs")
	}
	if interrupted > 0 {
		slog.InfoContext(ctx, "interrupted stale runs", slog.Int("count", interrupted))
	}

	for _, flow := range flows.Names() {
		if err := s.register(ctx, flow); err != nil {
			return err
		}
	}
	_, err = s.scheduler.NewJob(
		gocron.CronJob(pruneCron, false),
		gocron.NewTask(s.prune, context.WithoutCancel(ctx)),
		gocron.WithName(pruneName),
		gocron.WithTags(pruneName),
		gocron.WithSingletonMode(gocron.LimitModeReschedule),
	)
	if err != nil {
		return domain.Error(err, "register prune job")
	}
	s.scheduler.Start()
	return nil
}

func (s *Scheduler) Reload(ctx context.Context, flow flows.Name) error {
	if _, err := flows.ParseName(string(flow)); err != nil {
		return err
	}
	return s.register(ctx, flow)
}

func (s *Scheduler) Shutdown() error {
	if err := s.scheduler.Shutdown(); err != nil {
		return domain.Error(err, "shutdown scheduler")
	}
	return nil
}

func (s *Scheduler) register(ctx context.Context, flow flows.Name) error {
	flowAttr := slog.String("flow", string(flow))
	cron, err := s.settings.Schedule(ctx, flow)
	if err != nil {
		return domain.Error(err, "read flow schedule", flowAttr)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	job, err := s.scheduler.NewJob(
		gocron.CronJob(cron, false),
		gocron.NewTask(s.runFlow, context.WithoutCancel(ctx), flow),
		gocron.WithName(string(flow)),
		gocron.WithTags(string(flow)),
		gocron.WithSingletonMode(gocron.LimitModeReschedule),
	)
	if err != nil {
		return domain.Error(err, "register flow job", flowAttr, slog.String("schedule", cron))
	}
	if old, ok := s.jobs[flow]; ok {
		if err := s.scheduler.RemoveJob(old); err != nil {
			return domain.Error(err, "remove previous flow job", flowAttr)
		}
	}
	s.jobs[flow] = job.ID()
	return nil
}

func (s *Scheduler) runFlow(ctx context.Context, flow flows.Name) {
	flowAttr := slog.String("flow", string(flow))
	runID, err := s.runner.Run(ctx, flow, runs.TriggerSchedule)
	switch {
	case errors.Is(err, domain.ErrConflict):
		slog.InfoContext(ctx, "scheduled flow skipped: already running", flowAttr)
	case errors.Is(err, domain.ErrUpstream):
		domain.LogWarn(ctx, "scheduled flow failed", err, flowAttr)
	case err != nil:
		domain.LogError(ctx, "scheduled flow failed", err, flowAttr)
	default:
		slog.InfoContext(ctx, "scheduled flow finished", flowAttr, slog.String("run_id", runID))
	}
}

func (s *Scheduler) prune(ctx context.Context) {
	deleted, err := s.runs.PruneRuns(ctx, pruneRetention)
	if err != nil {
		domain.LogError(ctx, "prune runs failed", err)
		return
	}
	if deleted > 0 {
		slog.InfoContext(ctx, "pruned runs", slog.Int("deleted", deleted))
	}
}
