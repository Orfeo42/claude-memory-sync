package runs

import (
	"context"
	"log/slog"
	"time"

	"claude-memory-sync/internal/domain"
)

const defaultListLimit = 50

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) CreateRun(ctx context.Context, flow string, trigger Trigger) (Run, error) {
	run, err := s.repo.CreateRun(ctx, NewRun{Flow: flow, Trigger: trigger})
	if err != nil {
		return Run{}, domain.WrapError(err, slog.String("flow", flow))
	}

	slog.InfoContext(ctx, "run created",
		slog.String("run_id", run.ID),
		slog.String("flow", flow),
		slog.String("trigger", string(trigger)),
	)
	return run, nil
}

func (s *Service) AddStep(ctx context.Context, step NewStep) (Step, error) {
	if step.FinishedAt.Before(step.StartedAt) {
		return Step{}, domain.Error(domain.ErrValidation, "step finished before it started", slog.String("run_id", step.RunID))
	}

	added, err := s.repo.AddStep(ctx, step)
	if err != nil {
		return Step{}, domain.WrapError(err, slog.String("run_id", step.RunID))
	}

	slog.InfoContext(ctx, "run step added",
		slog.String("run_id", step.RunID),
		slog.String("step", step.Step),
		slog.String("step_status", string(step.Status)),
	)
	return added, nil
}

func (s *Service) FinishRun(ctx context.Context, f Finish) (Run, error) {
	if f.Status != StatusSucceeded && f.Status != StatusFailed && f.Status != StatusPartial {
		return Run{}, domain.Error(domain.ErrValidation, "run can only finish as succeeded, failed or partial", slog.String("run_id", f.ID))
	}

	run, err := s.repo.FinishRun(ctx, f)
	if err != nil {
		return Run{}, domain.WrapError(err, slog.String("run_id", f.ID))
	}

	slog.InfoContext(ctx, "run finished",
		slog.String("run_id", f.ID),
		slog.String("flow", run.Flow),
		slog.String("to_status", string(f.Status)),
	)
	return run, nil
}

func (s *Service) InterruptRuns(ctx context.Context, flow *string) (int, error) {
	n, err := s.repo.InterruptRuns(ctx, flow)
	if err != nil {
		return 0, domain.WrapError(err)
	}

	slog.InfoContext(ctx, "running runs interrupted",
		slog.Int("interrupted", n),
		slog.String("to_status", string(StatusInterrupted)),
	)
	return n, nil
}

func (s *Service) ListRuns(ctx context.Context, f ListFilter) ([]Run, error) {
	if f.Limit == 0 {
		f.Limit = defaultListLimit
	}
	list, err := s.repo.ListRuns(ctx, f)
	if err != nil {
		return nil, domain.WrapError(err)
	}
	return list, nil
}

func (s *Service) GetRun(ctx context.Context, id string) (RunDetail, error) {
	run, err := s.repo.GetRun(ctx, id)
	if err != nil {
		return RunDetail{}, domain.WrapError(err, slog.String("run_id", id))
	}
	steps, err := s.repo.Steps(ctx, id)
	if err != nil {
		return RunDetail{}, domain.WrapError(err, slog.String("run_id", id))
	}
	return RunDetail{Run: run, Steps: steps}, nil
}

func (s *Service) PruneRuns(ctx context.Context, olderThan time.Duration) (int, error) {
	if olderThan <= 0 {
		return 0, domain.Error(domain.ErrValidation, "older_than must be positive")
	}

	n, err := s.repo.PruneRuns(ctx, olderThan)
	if err != nil {
		return 0, domain.WrapError(err)
	}

	slog.InfoContext(ctx, "runs pruned", slog.Int("pruned", n))
	return n, nil
}
