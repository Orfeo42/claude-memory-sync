package synthsched

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/robfig/cron/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"claude-memory-sync/internal/domain"
	"claude-memory-sync/internal/flows"
	"claude-memory-sync/internal/runs"
)

type testError string

func (e testError) Error() string { return string(e) }

const errTest testError = "boom"

func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})))
	t.Cleanup(func() { slog.SetDefault(previous) })
	return &buf
}

func assertNextRun(t *testing.T, s *Scheduler, name string, expr string) {
	t.Helper()
	schedule, err := cron.ParseStandard(expr)
	require.NoError(t, err)
	for _, job := range s.scheduler.Jobs() {
		if job.Name() != name {
			continue
		}
		before := schedule.Next(time.Now())
		next, err := job.NextRun()
		require.NoError(t, err)
		after := schedule.Next(time.Now())
		assert.Contains(t, []time.Time{before, after}, next.In(before.Location()).Truncate(time.Second))
		return
	}
	t.Fatalf("job %s not found", name)
}

func newSettings(schedules map[flows.Name]string) *MockSettings {
	return &MockSettings{
		ScheduleFunc: func(_ context.Context, flow flows.Name) (string, error) {
			return schedules[flow], nil
		},
	}
}

func defaultSchedules() map[flows.Name]string {
	return map[flows.Name]string{
		flows.Intake:        "*/45 * * * *",
		flows.DailySynth:    "0 3 * * *",
		flows.Taskmine:      "0 4 * * 0",
		flows.ProposalApply: "*/10 * * * *",
	}
}

func newRuns() *MockRuns {
	return &MockRuns{
		InterruptRunsFunc: func(context.Context, string) (int, error) { return 0, nil },
		PruneRunsFunc:     func(context.Context, int) (int, error) { return 0, nil },
	}
}

func newStarted(t *testing.T, settings *MockSettings, runner *MockRunner, runRepo *MockRuns) *Scheduler {
	t.Helper()
	s, err := New(settings, runner, runRepo)
	require.NoError(t, err)
	require.NoError(t, s.Start(context.Background()))
	t.Cleanup(func() { assert.NoError(t, s.Shutdown()) })
	return s
}

func TestNew(t *testing.T) {
	t.Run("creates a scheduler", func(t *testing.T) {
		s, err := New(newSettings(nil), &MockRunner{}, newRuns())
		require.NoError(t, err)
		require.NotNil(t, s)
		require.NoError(t, s.Shutdown())
	})
}

func TestStart(t *testing.T) {
	t.Run("interrupts stale runs first and registers flows plus prune", func(t *testing.T) {
		settings := newSettings(defaultSchedules())
		runRepo := newRuns()
		s := newStarted(t, settings, &MockRunner{}, runRepo)

		require.Len(t, runRepo.InterruptRunsCalls(), 1)
		assert.Empty(t, runRepo.InterruptRunsCalls()[0].Flow)
		jobs := s.scheduler.Jobs()
		assert.Len(t, jobs, 5)
		names := make([]string, 0, len(jobs))
		for _, job := range jobs {
			names = append(names, job.Name())
			assert.Equal(t, []string{job.Name()}, job.Tags())
		}
		assert.ElementsMatch(t, []string{"intake", "daily-synth", "taskmine", "proposal-apply", pruneName}, names)
		for flow, expr := range defaultSchedules() {
			assertNextRun(t, s, string(flow), expr)
		}
		assertNextRun(t, s, pruneName, pruneCron)
	})

	t.Run("logs the interrupted count", func(t *testing.T) {
		buf := captureLogs(t)
		runRepo := newRuns()
		runRepo.InterruptRunsFunc = func(context.Context, string) (int, error) { return 3, nil }
		newStarted(t, newSettings(defaultSchedules()), &MockRunner{}, runRepo)

		assert.Contains(t, buf.String(), "interrupted stale runs")
		assert.Contains(t, buf.String(), "count=3")
	})

	t.Run("returns the interrupt error", func(t *testing.T) {
		runRepo := newRuns()
		runRepo.InterruptRunsFunc = func(context.Context, string) (int, error) { return 0, errTest }
		s, err := New(newSettings(defaultSchedules()), &MockRunner{}, runRepo)
		require.NoError(t, err)
		t.Cleanup(func() { assert.NoError(t, s.Shutdown()) })

		require.ErrorIs(t, s.Start(context.Background()), errTest)
		assert.Empty(t, s.scheduler.Jobs())
	})

	t.Run("returns a schedule read error", func(t *testing.T) {
		settings := newSettings(defaultSchedules())
		settings.ScheduleFunc = func(context.Context, flows.Name) (string, error) { return "", errTest }
		s, err := New(settings, &MockRunner{}, newRuns())
		require.NoError(t, err)
		t.Cleanup(func() { assert.NoError(t, s.Shutdown()) })

		require.ErrorIs(t, s.Start(context.Background()), errTest)
	})

	t.Run("returns an error for an invalid cron", func(t *testing.T) {
		schedules := defaultSchedules()
		schedules[flows.Taskmine] = "not a cron"
		s, err := New(newSettings(schedules), &MockRunner{}, newRuns())
		require.NoError(t, err)
		t.Cleanup(func() { assert.NoError(t, s.Shutdown()) })

		require.Error(t, s.Start(context.Background()))
	})
}

func TestReload(t *testing.T) {
	t.Run("replaces only that flow's job", func(t *testing.T) {
		schedules := defaultSchedules()
		s := newStarted(t, newSettings(schedules), &MockRunner{}, newRuns())
		previous := s.jobs[flows.Intake]
		other := s.jobs[flows.Taskmine]

		schedules[flows.Intake] = "*/5 * * * *"
		require.NoError(t, s.Reload(context.Background(), flows.Intake))

		assert.Len(t, s.scheduler.Jobs(), 5)
		assertNextRun(t, s, "intake", "*/5 * * * *")
		assert.NotEqual(t, previous, s.jobs[flows.Intake])
		assert.Equal(t, other, s.jobs[flows.Taskmine])
	})

	t.Run("rejects an unknown flow", func(t *testing.T) {
		s := newStarted(t, newSettings(defaultSchedules()), &MockRunner{}, newRuns())

		require.ErrorIs(t, s.Reload(context.Background(), flows.Name("nope")), domain.ErrValidation)
		assert.Len(t, s.scheduler.Jobs(), 5)
	})

	t.Run("keeps the old job on an invalid cron", func(t *testing.T) {
		schedules := defaultSchedules()
		s := newStarted(t, newSettings(schedules), &MockRunner{}, newRuns())

		schedules[flows.Intake] = "bad"
		require.Error(t, s.Reload(context.Background(), flows.Intake))

		assert.Len(t, s.scheduler.Jobs(), 5)
		assertNextRun(t, s, "intake", "*/45 * * * *")
	})

	t.Run("returns a schedule read error", func(t *testing.T) {
		settings := newSettings(defaultSchedules())
		s := newStarted(t, settings, &MockRunner{}, newRuns())

		settings.ScheduleFunc = func(context.Context, flows.Name) (string, error) { return "", errTest }
		require.ErrorIs(t, s.Reload(context.Background(), flows.Intake), errTest)
	})
}

func TestShutdown(t *testing.T) {
	t.Run("stops a started scheduler", func(t *testing.T) {
		s, err := New(newSettings(defaultSchedules()), &MockRunner{}, newRuns())
		require.NoError(t, err)
		require.NoError(t, s.Start(context.Background()))

		require.NoError(t, s.Shutdown())
	})
}

func TestRunFlow(t *testing.T) {
	t.Run("logs success with the run id", func(t *testing.T) {
		buf := captureLogs(t)
		runner := &MockRunner{RunFunc: func(context.Context, flows.Name, runs.Trigger) (string, error) { return "run-1", nil }}
		s := newStarted(t, newSettings(defaultSchedules()), runner, newRuns())

		s.runFlow(context.Background(), flows.Intake)

		require.Len(t, runner.RunCalls(), 1)
		assert.Equal(t, flows.Intake, runner.RunCalls()[0].Flow)
		assert.Equal(t, runs.TriggerSchedule, runner.RunCalls()[0].Trigger)
		assert.Contains(t, buf.String(), "level=INFO")
		assert.Contains(t, buf.String(), "run_id=run-1")
		assert.Contains(t, buf.String(), "flow=intake")
	})

	t.Run("logs a conflict at info", func(t *testing.T) {
		buf := captureLogs(t)
		runner := &MockRunner{RunFunc: func(context.Context, flows.Name, runs.Trigger) (string, error) {
			return "", domain.Error(domain.ErrConflict, "running")
		}}
		s := newStarted(t, newSettings(defaultSchedules()), runner, newRuns())

		s.runFlow(context.Background(), flows.Taskmine)

		assert.Contains(t, buf.String(), "level=INFO")
		assert.Contains(t, buf.String(), "scheduled flow skipped: already running")
		assert.NotContains(t, buf.String(), "level=ERROR")
	})

	t.Run("logs an upstream error at warn", func(t *testing.T) {
		buf := captureLogs(t)
		runner := &MockRunner{RunFunc: func(context.Context, flows.Name, runs.Trigger) (string, error) {
			return "", domain.Error(domain.ErrUpstream, "llm down")
		}}
		s := newStarted(t, newSettings(defaultSchedules()), runner, newRuns())

		s.runFlow(context.Background(), flows.Taskmine)

		assert.Contains(t, buf.String(), "level=WARN")
		assert.NotContains(t, buf.String(), "level=ERROR")
	})

	t.Run("logs any other error at error", func(t *testing.T) {
		buf := captureLogs(t)
		runner := &MockRunner{RunFunc: func(context.Context, flows.Name, runs.Trigger) (string, error) {
			return "", errors.New("unexpected")
		}}
		s := newStarted(t, newSettings(defaultSchedules()), runner, newRuns())

		s.runFlow(context.Background(), flows.Taskmine)

		assert.Contains(t, buf.String(), "level=ERROR")
	})
}

func TestPrune(t *testing.T) {
	t.Run("prunes with the retention and logs the count", func(t *testing.T) {
		buf := captureLogs(t)
		runRepo := newRuns()
		runRepo.PruneRunsFunc = func(context.Context, int) (int, error) { return 7, nil }
		s := newStarted(t, newSettings(defaultSchedules()), &MockRunner{}, runRepo)

		s.prune(context.Background())

		require.Len(t, runRepo.PruneRunsCalls(), 1)
		assert.Equal(t, 90, runRepo.PruneRunsCalls()[0].OlderThanDays)
		assert.Contains(t, buf.String(), "deleted=7")
	})

	t.Run("logs a prune error", func(t *testing.T) {
		buf := captureLogs(t)
		runRepo := newRuns()
		runRepo.PruneRunsFunc = func(context.Context, int) (int, error) { return 0, errTest }
		s := newStarted(t, newSettings(defaultSchedules()), &MockRunner{}, runRepo)

		s.prune(context.Background())

		assert.Contains(t, buf.String(), "level=ERROR")
	})
}
