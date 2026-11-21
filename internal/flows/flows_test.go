package flows

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"claude-memory-sync/internal/domain"
	"claude-memory-sync/internal/gitutil/gittest"
	"claude-memory-sync/internal/pipeline"
	"claude-memory-sync/internal/runs"
)

type recorded struct {
	mu       sync.Mutex
	steps    []StepResult
	finished int
	status   runs.Status
	errText  *string
	summary  json.RawMessage
}

func (r *recorded) stepNamed(name string) StepResult {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, s := range r.steps {
		if s.Step == name {
			return s
		}
	}
	return StepResult{}
}

func newRecorder() (*MockRecorder, *recorded) {
	rec := &recorded{}
	mock := &MockRecorder{
		StartFunc: func(context.Context, Name, runs.Trigger) (string, error) { return "run-1", nil },
		StepFunc: func(_ context.Context, _ string, step StepResult) error {
			rec.mu.Lock()
			defer rec.mu.Unlock()
			rec.steps = append(rec.steps, step)
			return nil
		},
		FinishFunc: func(_ context.Context, _ string, status runs.Status, errText *string, summary json.RawMessage) error {
			rec.mu.Lock()
			defer rec.mu.Unlock()
			rec.finished++
			rec.status = status
			rec.errText = errText
			rec.summary = summary
			return nil
		},
	}
	return mock, rec
}

func newIdlePipeline() *MockPipeline {
	return &MockPipeline{
		ExpireFunc: func(context.Context, string, int) (int, error) { return 0, nil },
		ClaimFunc: func(context.Context, string, int) (pipeline.Proposal, error) {
			return pipeline.Proposal{}, pipeline.ErrNothingToClaim
		},
	}
}

func newTestConfig(t *testing.T, seed map[string]string) Config {
	t.Helper()

	reposDir := filepath.Join(t.TempDir(), "repos")
	bare := filepath.Join(reposDir, "canonical.git")
	gittest.InitBareRepo(t, bare)
	seed["README.md"] = "seed\n"
	gittest.CommitFiles(t, bare, seed, "seed", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))

	return Config{
		ReposDir: reposDir,
		WorkDir:  filepath.Join(t.TempDir(), "work"),
		Lock:     filepath.Join(t.TempDir(), "lock"),
		Worker:   "w1",
	}
}

func TestNames(t *testing.T) {
	t.Run("lists the four flows in order", func(t *testing.T) {
		assert.Equal(t, []Name{Intake, DailySynth, Taskmine, ProposalApply}, Names())
	})
}

func TestParseName(t *testing.T) {
	t.Run("known flow", func(t *testing.T) {
		name, err := ParseName("daily-synth")
		require.NoError(t, err)
		assert.Equal(t, DailySynth, name)
	})

	t.Run("unknown flow is a validation error", func(t *testing.T) {
		_, err := ParseName("nope")
		require.ErrorIs(t, err, domain.ErrValidation)
	})
}

func TestRunnerRun(t *testing.T) {
	t.Run("unknown flow is a validation error", func(t *testing.T) {
		recorder, _ := newRecorder()
		r := New(Config{}, Deps{Recorder: recorder})

		_, err := r.Run(t.Context(), Name("nope"), runs.TriggerManual)
		require.ErrorIs(t, err, domain.ErrValidation)
		assert.Empty(t, recorder.StartCalls())
	})

	t.Run("second run of the same flow conflicts while the first is blocked", func(t *testing.T) {
		recorder, _ := newRecorder()
		started := make(chan struct{})
		release := make(chan struct{})
		recorder.StartFunc = func(context.Context, Name, runs.Trigger) (string, error) {
			close(started)
			<-release
			return "run-1", nil
		}
		r := New(newTestConfig(t, map[string]string{}), Deps{Recorder: recorder, Pipeline: newIdlePipeline()})

		done := make(chan error, 1)
		go func() {
			_, err := r.Run(t.Context(), ProposalApply, runs.TriggerManual)
			done <- err
		}()
		<-started

		assert.True(t, r.Running(ProposalApply))
		_, err := r.Run(t.Context(), ProposalApply, runs.TriggerManual)
		require.ErrorIs(t, err, domain.ErrConflict)

		close(release)
		require.NoError(t, <-done)
		assert.False(t, r.Running(ProposalApply))
	})

	t.Run("recorder start error is returned and finish is not called", func(t *testing.T) {
		recorder, rec := newRecorder()
		recorder.StartFunc = func(context.Context, Name, runs.Trigger) (string, error) {
			return "", domain.Error(domain.ErrUpstream, "start failed")
		}
		r := New(Config{}, Deps{Recorder: recorder})

		_, err := r.Run(t.Context(), ProposalApply, runs.TriggerManual)
		require.ErrorIs(t, err, domain.ErrUpstream)
		assert.Zero(t, rec.finished)
		assert.False(t, r.Running(ProposalApply))
	})

	t.Run("body error finishes failed with the error text", func(t *testing.T) {
		recorder, rec := newRecorder()
		cfg := Config{ReposDir: t.TempDir(), WorkDir: filepath.Join(t.TempDir(), "work"), Lock: filepath.Join(t.TempDir(), "lock")}
		r := New(cfg, Deps{Recorder: recorder, Pipeline: newIdlePipeline()})

		runID, err := r.Run(t.Context(), ProposalApply, runs.TriggerManual)
		require.Error(t, err)
		assert.Equal(t, "run-1", runID)
		assert.Equal(t, 1, rec.finished)
		assert.Equal(t, runs.StatusFailed, rec.status)
		require.NotNil(t, rec.errText)
		assert.NotEmpty(t, *rec.errText)
		assert.Equal(t, runs.StepFailed, rec.stepNamed("ensure_work").Status)
	})

	t.Run("failed step with nil body error finishes partial", func(t *testing.T) {
		recorder, rec := newRecorder()
		cfg := newTestConfig(t, map[string]string{})
		p := newIdlePipeline()
		claims := []pipeline.Proposal{secretRuleProposal("bad")}
		p.ClaimFunc = func(context.Context, string, int) (pipeline.Proposal, error) {
			if len(claims) == 0 {
				return pipeline.Proposal{}, pipeline.ErrNothingToClaim
			}
			next := claims[0]
			claims = claims[1:]
			return next, nil
		}
		p.MarkFailedFunc = func(_ context.Context, id, _, _ string) (pipeline.Proposal, error) {
			return pipeline.Proposal{ID: id}, nil
		}
		r := New(cfg, Deps{Recorder: recorder, Pipeline: p})

		_, err := r.Run(t.Context(), ProposalApply, runs.TriggerManual)
		require.NoError(t, err)
		assert.Equal(t, runs.StatusPartial, rec.status)
		assert.Nil(t, rec.errText)
	})

	t.Run("clean run finishes succeeded with the summary json", func(t *testing.T) {
		recorder, rec := newRecorder()
		r := New(newTestConfig(t, map[string]string{}), Deps{Recorder: recorder, Pipeline: newIdlePipeline()})

		_, err := r.Run(t.Context(), ProposalApply, runs.TriggerManual)
		require.NoError(t, err)
		assert.Equal(t, runs.StatusSucceeded, rec.status)
		assert.Nil(t, rec.errText)
		assert.JSONEq(t, `{"applied":0,"failed":0}`, string(rec.summary))
	})

	t.Run("finish error is returned", func(t *testing.T) {
		recorder, _ := newRecorder()
		recorder.FinishFunc = func(context.Context, string, runs.Status, *string, json.RawMessage) error {
			return domain.Error(domain.ErrUpstream, "finish failed")
		}
		r := New(newTestConfig(t, map[string]string{}), Deps{Recorder: recorder, Pipeline: newIdlePipeline()})

		_, err := r.Run(t.Context(), ProposalApply, runs.TriggerManual)
		require.ErrorIs(t, err, domain.ErrUpstream)
	})
}

func TestRunnerRunning(t *testing.T) {
	t.Run("false when idle", func(t *testing.T) {
		r := New(Config{}, Deps{})
		assert.False(t, r.Running(Intake))
	})

	t.Run("true after acquire and false after release", func(t *testing.T) {
		r := New(Config{}, Deps{})
		require.True(t, r.acquire(Intake))
		assert.True(t, r.Running(Intake))
		assert.False(t, r.Running(Taskmine))
		r.release(Intake)
		assert.False(t, r.Running(Intake))
	})
}

func TestTail(t *testing.T) {
	t.Run("empty output is nil", func(t *testing.T) {
		assert.Nil(t, tail("  \n"))
	})

	t.Run("short output is kept trimmed", func(t *testing.T) {
		got := tail(" hello \n")
		require.NotNil(t, got)
		assert.Equal(t, "hello", *got)
	})

	t.Run("long output keeps the last 8000 bytes", func(t *testing.T) {
		got := tail(strings.Repeat("a", 5000) + strings.Repeat("b", 8000))
		require.NotNil(t, got)
		assert.Equal(t, strings.Repeat("b", 8000), *got)
	})

	t.Run("cut inside a multibyte rune stays valid utf8", func(t *testing.T) {
		got := tail(strings.Repeat("€", 4000))
		require.NotNil(t, got)
		assert.True(t, utf8.ValidString(*got))
		assert.LessOrEqual(t, len(*got), maxOutputTail)
		assert.True(t, strings.HasPrefix(*got, "€"))
	})
}
