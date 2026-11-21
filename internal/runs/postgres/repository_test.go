package postgres

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"claude-memory-sync/internal/domain"
	"claude-memory-sync/internal/runs"
)

var t0 = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func stepFor(runID, name string) runs.NewStep {
	return runs.NewStep{RunID: runID, Step: name, Status: runs.StepSucceeded, StartedAt: t0, FinishedAt: t0.Add(time.Second)}
}

func TestRepositoryCreateRun(t *testing.T) {
	t.Run("creates a running run with empty summary", func(t *testing.T) {
		ctx := t.Context()
		repo := setup(ctx, t)

		run := mustCreate(ctx, t, repo, "daily-synth")
		assert.NotEmpty(t, run.ID)
		assert.Equal(t, "daily-synth", run.Flow)
		assert.Equal(t, runs.TriggerSchedule, run.Trigger)
		assert.Equal(t, runs.StatusRunning, run.Status)
		assert.Nil(t, run.FinishedAt)
		assert.JSONEq(t, `{}`, string(run.Summary))
	})
}

func TestRepositoryAddStep(t *testing.T) {
	t.Run("adds a step to a running run", func(t *testing.T) {
		ctx := t.Context()
		repo := setup(ctx, t)
		run := mustCreate(ctx, t, repo, "intake")
		tail := "tail"

		step := stepFor(run.ID, "scan")
		step.OutputTail = &tail
		got, err := repo.AddStep(ctx, step)
		require.NoError(t, err)
		assert.Equal(t, run.ID, got.RunID)
		assert.Equal(t, "scan", got.Step)
		assert.Equal(t, runs.StepSucceeded, got.Status)
		assert.Equal(t, &tail, got.OutputTail)
		assert.Nil(t, got.Error)
	})

	t.Run("unknown run is not found", func(t *testing.T) {
		ctx := t.Context()
		repo := setup(ctx, t)

		_, err := repo.AddStep(ctx, stepFor("00000000-0000-0000-0000-000000000000", "scan"))
		require.ErrorIs(t, err, domain.ErrNotFound)
	})

	t.Run("malformed run id is not found", func(t *testing.T) {
		ctx := t.Context()
		repo := setup(ctx, t)

		_, err := repo.AddStep(ctx, stepFor("not-a-uuid", "scan"))
		require.ErrorIs(t, err, domain.ErrNotFound)
	})

	t.Run("finished run is a conflict", func(t *testing.T) {
		ctx := t.Context()
		repo := setup(ctx, t)
		run := mustCreate(ctx, t, repo, "intake")
		mustFinish(ctx, t, repo, run.ID, runs.StatusSucceeded)

		_, err := repo.AddStep(ctx, stepFor(run.ID, "scan"))
		require.ErrorIs(t, err, domain.ErrConflict)
	})
}

func TestRepositoryFinishRun(t *testing.T) {
	t.Run("finishes a running run and stores the summary", func(t *testing.T) {
		ctx := t.Context()
		repo := setup(ctx, t)
		run := mustCreate(ctx, t, repo, "intake")
		errText := "one step failed"

		got, err := repo.FinishRun(ctx, runs.Finish{ID: run.ID, Status: runs.StatusPartial, Error: &errText, Summary: json.RawMessage(`{"proposals":2}`)})
		require.NoError(t, err)
		assert.Equal(t, runs.StatusPartial, got.Status)
		assert.NotNil(t, got.FinishedAt)
		assert.Equal(t, &errText, got.Error)
		assert.JSONEq(t, `{"proposals":2}`, string(got.Summary))
	})

	t.Run("missing summary keeps the existing one", func(t *testing.T) {
		ctx := t.Context()
		repo := setup(ctx, t)
		run := mustCreate(ctx, t, repo, "intake")

		got := mustFinish(ctx, t, repo, run.ID, runs.StatusSucceeded)
		assert.JSONEq(t, `{}`, string(got.Summary))
	})

	t.Run("second finish is a conflict", func(t *testing.T) {
		ctx := t.Context()
		repo := setup(ctx, t)
		run := mustCreate(ctx, t, repo, "intake")
		mustFinish(ctx, t, repo, run.ID, runs.StatusSucceeded)

		_, err := repo.FinishRun(ctx, runs.Finish{ID: run.ID, Status: runs.StatusFailed})
		require.ErrorIs(t, err, domain.ErrConflict)

		got, err := repo.GetRun(ctx, run.ID)
		require.NoError(t, err)
		assert.Equal(t, runs.StatusSucceeded, got.Status)
	})

	t.Run("unknown run is not found", func(t *testing.T) {
		ctx := t.Context()
		repo := setup(ctx, t)

		_, err := repo.FinishRun(ctx, runs.Finish{ID: "00000000-0000-0000-0000-000000000000", Status: runs.StatusFailed})
		require.ErrorIs(t, err, domain.ErrNotFound)
	})
}

func TestRepositoryInterruptRuns(t *testing.T) {
	t.Run("interrupts only the running runs of one flow", func(t *testing.T) {
		ctx := t.Context()
		repo := setup(ctx, t)
		target := mustCreate(ctx, t, repo, "intake")
		other := mustCreate(ctx, t, repo, "taskmine")
		done := mustCreate(ctx, t, repo, "intake")
		mustFinish(ctx, t, repo, done.ID, runs.StatusSucceeded)
		flow := "intake"

		n, err := repo.InterruptRuns(ctx, &flow)
		require.NoError(t, err)
		assert.Equal(t, 1, n)

		got, err := repo.GetRun(ctx, target.ID)
		require.NoError(t, err)
		assert.Equal(t, runs.StatusInterrupted, got.Status)
		assert.NotNil(t, got.FinishedAt)
		require.NotNil(t, got.Error)
		assert.Equal(t, "interrupted: synth restarted", *got.Error)

		got, err = repo.GetRun(ctx, other.ID)
		require.NoError(t, err)
		assert.Equal(t, runs.StatusRunning, got.Status)

		got, err = repo.GetRun(ctx, done.ID)
		require.NoError(t, err)
		assert.Equal(t, runs.StatusSucceeded, got.Status)
	})

	t.Run("nil flow interrupts every running run", func(t *testing.T) {
		ctx := t.Context()
		repo := setup(ctx, t)
		mustCreate(ctx, t, repo, "intake")
		mustCreate(ctx, t, repo, "taskmine")

		n, err := repo.InterruptRuns(ctx, nil)
		require.NoError(t, err)
		assert.Equal(t, 2, n)
	})

	t.Run("nothing running returns zero", func(t *testing.T) {
		ctx := t.Context()
		repo := setup(ctx, t)

		n, err := repo.InterruptRuns(ctx, nil)
		require.NoError(t, err)
		assert.Equal(t, 0, n)
	})
}

func TestRepositoryListRuns(t *testing.T) {
	t.Run("returns newest first and filters by flow", func(t *testing.T) {
		ctx := t.Context()
		repo := setup(ctx, t)
		oldest := mustCreate(ctx, t, repo, "intake")
		newest := mustCreate(ctx, t, repo, "intake")
		mustCreate(ctx, t, repo, "taskmine")
		backdateStarted(ctx, t, oldest.ID, 2*time.Hour)
		flow := "intake"

		got, err := repo.ListRuns(ctx, runs.ListFilter{Flow: &flow, Limit: 10})
		require.NoError(t, err)
		require.Len(t, got, 2)
		assert.Equal(t, newest.ID, got[0].ID)
		assert.Equal(t, oldest.ID, got[1].ID)
	})

	t.Run("filters by status", func(t *testing.T) {
		ctx := t.Context()
		repo := setup(ctx, t)
		failed := mustCreate(ctx, t, repo, "intake")
		mustCreate(ctx, t, repo, "intake")
		mustFinish(ctx, t, repo, failed.ID, runs.StatusFailed)
		status := runs.StatusFailed

		got, err := repo.ListRuns(ctx, runs.ListFilter{Status: &status, Limit: 10})
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Equal(t, failed.ID, got[0].ID)
	})

	t.Run("before cursor and limit page the result", func(t *testing.T) {
		ctx := t.Context()
		repo := setup(ctx, t)
		old := mustCreate(ctx, t, repo, "intake")
		mustCreate(ctx, t, repo, "intake")
		backdateStarted(ctx, t, old.ID, 2*time.Hour)
		before := time.Now().Add(-time.Hour)

		got, err := repo.ListRuns(ctx, runs.ListFilter{Before: &before, Limit: 10})
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Equal(t, old.ID, got[0].ID)

		got, err = repo.ListRuns(ctx, runs.ListFilter{Limit: 1})
		require.NoError(t, err)
		assert.Len(t, got, 1)
	})

	t.Run("empty result is an empty slice", func(t *testing.T) {
		ctx := t.Context()
		repo := setup(ctx, t)

		got, err := repo.ListRuns(ctx, runs.ListFilter{Limit: 10})
		require.NoError(t, err)
		assert.NotNil(t, got)
		assert.Empty(t, got)
	})
}

func TestRepositoryGetRun(t *testing.T) {
	t.Run("returns the run", func(t *testing.T) {
		ctx := t.Context()
		repo := setup(ctx, t)
		run := mustCreate(ctx, t, repo, "intake")

		got, err := repo.GetRun(ctx, run.ID)
		require.NoError(t, err)
		assert.Equal(t, run.ID, got.ID)
	})

	t.Run("unknown id is not found", func(t *testing.T) {
		ctx := t.Context()
		repo := setup(ctx, t)

		_, err := repo.GetRun(ctx, "00000000-0000-0000-0000-000000000000")
		require.ErrorIs(t, err, domain.ErrNotFound)
	})

	t.Run("malformed id is not found", func(t *testing.T) {
		ctx := t.Context()
		repo := setup(ctx, t)

		_, err := repo.GetRun(ctx, "nope")
		require.ErrorIs(t, err, domain.ErrNotFound)
	})
}

func TestRepositorySteps(t *testing.T) {
	t.Run("returns steps ordered by id", func(t *testing.T) {
		ctx := t.Context()
		repo := setup(ctx, t)
		run := mustCreate(ctx, t, repo, "intake")
		_, err := repo.AddStep(ctx, stepFor(run.ID, "second-inserted-first"))
		require.NoError(t, err)
		_, err = repo.AddStep(ctx, stepFor(run.ID, "b"))
		require.NoError(t, err)

		got, err := repo.Steps(ctx, run.ID)
		require.NoError(t, err)
		require.Len(t, got, 2)
		assert.Equal(t, "second-inserted-first", got[0].Step)
		assert.Equal(t, "b", got[1].Step)
		assert.Less(t, got[0].ID, got[1].ID)
	})

	t.Run("run without steps returns an empty slice", func(t *testing.T) {
		ctx := t.Context()
		repo := setup(ctx, t)
		run := mustCreate(ctx, t, repo, "intake")

		got, err := repo.Steps(ctx, run.ID)
		require.NoError(t, err)
		assert.NotNil(t, got)
		assert.Empty(t, got)
	})
}

func TestRepositoryPruneRuns(t *testing.T) {
	t.Run("deletes old finished runs and cascades steps", func(t *testing.T) {
		ctx := t.Context()
		repo := setup(ctx, t)
		old := mustCreate(ctx, t, repo, "intake")
		_, err := repo.AddStep(ctx, stepFor(old.ID, "scan"))
		require.NoError(t, err)
		mustFinish(ctx, t, repo, old.ID, runs.StatusSucceeded)
		backdateStarted(ctx, t, old.ID, 100*24*time.Hour)

		n, err := repo.PruneRuns(ctx, 90*24*time.Hour)
		require.NoError(t, err)
		assert.Equal(t, 1, n)

		_, err = repo.GetRun(ctx, old.ID)
		require.ErrorIs(t, err, domain.ErrNotFound)
		var steps int
		require.NoError(t, testDB.GetContext(ctx, &steps, "SELECT count(*) FROM run_steps"))
		assert.Equal(t, 0, steps)
	})

	t.Run("keeps old running runs", func(t *testing.T) {
		ctx := t.Context()
		repo := setup(ctx, t)
		running := mustCreate(ctx, t, repo, "intake")
		backdateStarted(ctx, t, running.ID, 100*24*time.Hour)

		n, err := repo.PruneRuns(ctx, 90*24*time.Hour)
		require.NoError(t, err)
		assert.Equal(t, 0, n)
	})

	t.Run("keeps recent finished runs", func(t *testing.T) {
		ctx := t.Context()
		repo := setup(ctx, t)
		recent := mustCreate(ctx, t, repo, "intake")
		mustFinish(ctx, t, repo, recent.ID, runs.StatusFailed)

		n, err := repo.PruneRuns(ctx, 90*24*time.Hour)
		require.NoError(t, err)
		assert.Equal(t, 0, n)
	})
}
