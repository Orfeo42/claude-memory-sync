package runs

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"claude-memory-sync/internal/domain"
)

var t0 = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func TestServiceCreateRun(t *testing.T) {
	t.Run("passes flow and trigger to the repository", func(t *testing.T) {
		repo := &MockRepository{
			CreateRunFunc: func(_ context.Context, r NewRun) (Run, error) {
				return Run{ID: "r1", Flow: r.Flow, Trigger: r.Trigger, Status: StatusRunning}, nil
			},
		}

		got, err := NewService(repo).CreateRun(t.Context(), "intake", TriggerManual)
		require.NoError(t, err)
		assert.Equal(t, "r1", got.ID)
		assert.Equal(t, NewRun{Flow: "intake", Trigger: TriggerManual}, repo.CreateRunCalls()[0].R)
	})

	t.Run("repository error is wrapped", func(t *testing.T) {
		repo := &MockRepository{
			CreateRunFunc: func(_ context.Context, _ NewRun) (Run, error) { return Run{}, domain.ErrInternal },
		}

		_, err := NewService(repo).CreateRun(t.Context(), "intake", TriggerManual)
		require.ErrorIs(t, err, domain.ErrInternal)
	})
}

func TestServiceAddStep(t *testing.T) {
	t.Run("stores a valid step", func(t *testing.T) {
		repo := &MockRepository{
			AddStepFunc: func(_ context.Context, s NewStep) (Step, error) {
				return Step{ID: 1, RunID: s.RunID, Step: s.Step, Status: s.Status}, nil
			},
		}

		got, err := NewService(repo).AddStep(t.Context(), NewStep{RunID: "r1", Step: "scan", Status: StepSucceeded, StartedAt: t0, FinishedAt: t0.Add(time.Second)})
		require.NoError(t, err)
		assert.Equal(t, int64(1), got.ID)
	})

	t.Run("finish before start is a validation error", func(t *testing.T) {
		repo := &MockRepository{}

		_, err := NewService(repo).AddStep(t.Context(), NewStep{RunID: "r1", StartedAt: t0, FinishedAt: t0.Add(-time.Second)})
		require.ErrorIs(t, err, domain.ErrValidation)
		assert.Empty(t, repo.AddStepCalls())
	})

	t.Run("conflict from the repository is preserved", func(t *testing.T) {
		repo := &MockRepository{
			AddStepFunc: func(_ context.Context, _ NewStep) (Step, error) {
				return Step{}, domain.Error(domain.ErrConflict, "run is not running")
			},
		}

		_, err := NewService(repo).AddStep(t.Context(), NewStep{RunID: "r1", StartedAt: t0, FinishedAt: t0})
		require.ErrorIs(t, err, domain.ErrConflict)
	})
}

func TestServiceFinishRun(t *testing.T) {
	t.Run("finishes with a terminal status", func(t *testing.T) {
		repo := &MockRepository{
			FinishRunFunc: func(_ context.Context, f Finish) (Run, error) {
				return Run{ID: f.ID, Status: f.Status}, nil
			},
		}

		got, err := NewService(repo).FinishRun(t.Context(), Finish{ID: "r1", Status: StatusPartial})
		require.NoError(t, err)
		assert.Equal(t, StatusPartial, got.Status)
	})

	t.Run("running is rejected", func(t *testing.T) {
		repo := &MockRepository{}

		_, err := NewService(repo).FinishRun(t.Context(), Finish{ID: "r1", Status: StatusRunning})
		require.ErrorIs(t, err, domain.ErrValidation)
		assert.Empty(t, repo.FinishRunCalls())
	})

	t.Run("interrupted is rejected", func(t *testing.T) {
		_, err := NewService(&MockRepository{}).FinishRun(t.Context(), Finish{ID: "r1", Status: StatusInterrupted})
		require.ErrorIs(t, err, domain.ErrValidation)
	})

	t.Run("unknown status is rejected", func(t *testing.T) {
		_, err := NewService(&MockRepository{}).FinishRun(t.Context(), Finish{ID: "r1", Status: Status("paused")})
		require.ErrorIs(t, err, domain.ErrValidation)
	})

	t.Run("not found from the repository is preserved", func(t *testing.T) {
		repo := &MockRepository{
			FinishRunFunc: func(_ context.Context, _ Finish) (Run, error) {
				return Run{}, domain.Error(domain.ErrNotFound, "run not found")
			},
		}

		_, err := NewService(repo).FinishRun(t.Context(), Finish{ID: "r1", Status: StatusFailed})
		require.ErrorIs(t, err, domain.ErrNotFound)
	})
}

func TestServiceInterruptRuns(t *testing.T) {
	t.Run("returns the interrupted count", func(t *testing.T) {
		flow := "intake"
		repo := &MockRepository{
			InterruptRunsFunc: func(_ context.Context, _ *string) (int, error) { return 3, nil },
		}

		n, err := NewService(repo).InterruptRuns(t.Context(), &flow)
		require.NoError(t, err)
		assert.Equal(t, 3, n)
		assert.Equal(t, &flow, repo.InterruptRunsCalls()[0].Flow)
	})

	t.Run("repository error is wrapped", func(t *testing.T) {
		repo := &MockRepository{
			InterruptRunsFunc: func(_ context.Context, _ *string) (int, error) { return 0, domain.ErrInternal },
		}

		_, err := NewService(repo).InterruptRuns(t.Context(), nil)
		require.ErrorIs(t, err, domain.ErrInternal)
	})
}

func TestServiceListRuns(t *testing.T) {
	t.Run("defaults the limit", func(t *testing.T) {
		repo := &MockRepository{
			ListRunsFunc: func(_ context.Context, _ ListFilter) ([]Run, error) { return []Run{{ID: "r1"}}, nil },
		}

		got, err := NewService(repo).ListRuns(t.Context(), ListFilter{})
		require.NoError(t, err)
		assert.Len(t, got, 1)
		assert.Equal(t, defaultListLimit, repo.ListRunsCalls()[0].F.Limit)
	})

	t.Run("keeps an explicit limit", func(t *testing.T) {
		repo := &MockRepository{
			ListRunsFunc: func(_ context.Context, _ ListFilter) ([]Run, error) { return nil, nil },
		}

		_, err := NewService(repo).ListRuns(t.Context(), ListFilter{Limit: 7})
		require.NoError(t, err)
		assert.Equal(t, 7, repo.ListRunsCalls()[0].F.Limit)
	})

	t.Run("repository error is wrapped", func(t *testing.T) {
		repo := &MockRepository{
			ListRunsFunc: func(_ context.Context, _ ListFilter) ([]Run, error) { return nil, domain.ErrInternal },
		}

		_, err := NewService(repo).ListRuns(t.Context(), ListFilter{})
		require.ErrorIs(t, err, domain.ErrInternal)
	})
}

func TestServiceGetRun(t *testing.T) {
	t.Run("combines run and steps", func(t *testing.T) {
		repo := &MockRepository{
			GetRunFunc: func(_ context.Context, id string) (Run, error) { return Run{ID: id}, nil },
			StepsFunc:  func(_ context.Context, _ string) ([]Step, error) { return []Step{{ID: 1}, {ID: 2}}, nil },
		}

		got, err := NewService(repo).GetRun(t.Context(), "r1")
		require.NoError(t, err)
		assert.Equal(t, "r1", got.Run.ID)
		assert.Len(t, got.Steps, 2)
	})

	t.Run("unknown run is not found", func(t *testing.T) {
		repo := &MockRepository{
			GetRunFunc: func(_ context.Context, _ string) (Run, error) {
				return Run{}, domain.Error(domain.ErrNotFound, "run not found")
			},
		}

		_, err := NewService(repo).GetRun(t.Context(), "nope")
		require.ErrorIs(t, err, domain.ErrNotFound)
		assert.Empty(t, repo.StepsCalls())
	})

	t.Run("steps error is wrapped", func(t *testing.T) {
		repo := &MockRepository{
			GetRunFunc: func(_ context.Context, id string) (Run, error) { return Run{ID: id}, nil },
			StepsFunc:  func(_ context.Context, _ string) ([]Step, error) { return nil, domain.ErrInternal },
		}

		_, err := NewService(repo).GetRun(t.Context(), "r1")
		require.ErrorIs(t, err, domain.ErrInternal)
	})
}

func TestServicePruneRuns(t *testing.T) {
	t.Run("returns the pruned count", func(t *testing.T) {
		repo := &MockRepository{
			PruneRunsFunc: func(_ context.Context, _ time.Duration) (int, error) { return 5, nil },
		}

		n, err := NewService(repo).PruneRuns(t.Context(), 24*time.Hour)
		require.NoError(t, err)
		assert.Equal(t, 5, n)
		assert.Equal(t, 24*time.Hour, repo.PruneRunsCalls()[0].OlderThan)
	})

	t.Run("non-positive duration is rejected", func(t *testing.T) {
		repo := &MockRepository{}

		_, err := NewService(repo).PruneRuns(t.Context(), 0)
		require.ErrorIs(t, err, domain.ErrValidation)
		assert.Empty(t, repo.PruneRunsCalls())
	})

	t.Run("repository error is wrapped", func(t *testing.T) {
		repo := &MockRepository{
			PruneRunsFunc: func(_ context.Context, _ time.Duration) (int, error) { return 0, domain.ErrInternal },
		}

		_, err := NewService(repo).PruneRuns(t.Context(), time.Hour)
		require.ErrorIs(t, err, domain.ErrInternal)
	})
}
