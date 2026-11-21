package client

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"claude-memory-sync/internal/domain"
	"claude-memory-sync/internal/runs"
)

const runJSON = `{"id":"r1","flow":"daily-synth","trigger":"schedule","status":"running","started_at":"2026-01-01T00:00:00Z","summary":{}}`

func TestCreateRun(t *testing.T) {
	t.Run("posts body and returns run", func(t *testing.T) {
		c, rec := fakeServer(t, http.StatusCreated, runJSON)

		got, err := c.CreateRun(t.Context(), CreateRunRequest{Flow: "daily-synth", Trigger: runs.TriggerSchedule})
		require.NoError(t, err)
		assert.Equal(t, "r1", got.ID)
		assert.Equal(t, runs.StatusRunning, got.Status)
		assert.Equal(t, http.MethodPost, rec.method)
		assert.Equal(t, "/v1/runs", rec.path)
		assert.Equal(t, "Bearer "+testToken, rec.auth)
		assert.Equal(t, "daily-synth", rec.body["flow"])
		assert.Equal(t, "schedule", rec.body["trigger"])
	})

	t.Run("400 maps to validation", func(t *testing.T) {
		c, _ := fakeServer(t, http.StatusBadRequest, `{"detail":"bad"}`)

		_, err := c.CreateRun(t.Context(), CreateRunRequest{})
		require.ErrorIs(t, err, domain.ErrValidation)
	})
}

func TestAddRunStep(t *testing.T) {
	t.Run("posts step to run path", func(t *testing.T) {
		c, rec := fakeServer(t, http.StatusCreated, `{"id":3,"run_id":"r1","step":"scan","status":"succeeded","started_at":"2026-01-01T00:00:00Z","finished_at":"2026-01-01T00:00:01Z"}`)
		start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

		got, err := c.AddRunStep(t.Context(), "r1", AddRunStepRequest{Step: "scan", Status: runs.StepSucceeded, StartedAt: start, FinishedAt: start.Add(time.Second)})
		require.NoError(t, err)
		assert.Equal(t, int64(3), got.ID)
		assert.Equal(t, "/v1/runs/r1/steps", rec.path)
		assert.Equal(t, "scan", rec.body["step"])
		assert.Equal(t, "succeeded", rec.body["status"])
	})

	t.Run("409 maps to conflict", func(t *testing.T) {
		c, _ := fakeServer(t, http.StatusConflict, `{"detail":"run is not running"}`)

		_, err := c.AddRunStep(t.Context(), "r1", AddRunStepRequest{})
		require.ErrorIs(t, err, domain.ErrConflict)
	})
}

func TestFinishRun(t *testing.T) {
	t.Run("posts status and summary", func(t *testing.T) {
		c, rec := fakeServer(t, http.StatusOK, runJSON)

		_, err := c.FinishRun(t.Context(), "r1", FinishRunRequest{Status: runs.StatusPartial, Summary: []byte(`{"k":1}`)})
		require.NoError(t, err)
		assert.Equal(t, "/v1/runs/r1/finish", rec.path)
		assert.Equal(t, "partial", rec.body["status"])
		assert.Equal(t, map[string]any{"k": float64(1)}, rec.body["summary"])
	})

	t.Run("404 maps to not found", func(t *testing.T) {
		c, _ := fakeServer(t, http.StatusNotFound, `{"detail":"missing"}`)

		_, err := c.FinishRun(t.Context(), "nope", FinishRunRequest{Status: runs.StatusFailed})
		require.ErrorIs(t, err, domain.ErrNotFound)
	})
}

func TestInterruptRuns(t *testing.T) {
	t.Run("sends flow and returns count", func(t *testing.T) {
		c, rec := fakeServer(t, http.StatusOK, `{"count":2}`)

		n, err := c.InterruptRuns(t.Context(), "taskmine")
		require.NoError(t, err)
		assert.Equal(t, 2, n)
		assert.Equal(t, "/v1/runs/interrupt", rec.path)
		assert.Equal(t, "taskmine", rec.body["flow"])
	})

	t.Run("empty flow omits the field", func(t *testing.T) {
		c, rec := fakeServer(t, http.StatusOK, `{"count":0}`)

		_, err := c.InterruptRuns(t.Context(), "")
		require.NoError(t, err)
		assert.NotContains(t, rec.body, "flow")
	})
}

func TestListRuns(t *testing.T) {
	t.Run("sends filters and decodes wrapped runs", func(t *testing.T) {
		c, rec := fakeServer(t, http.StatusOK, `{"runs":[`+runJSON+`]}`)
		status := runs.StatusFailed
		before := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)

		got, err := c.ListRuns(t.Context(), ListRunsParams{Flow: "intake", Status: &status, Before: &before, Limit: 5})
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Equal(t, http.MethodGet, rec.method)
		assert.Equal(t, "/v1/runs", rec.path)
		assert.Contains(t, rec.query, "flow=intake")
		assert.Contains(t, rec.query, "status=failed")
		assert.Contains(t, rec.query, "limit=5")
		assert.Contains(t, rec.query, "before=2026-01-02T00%3A00%3A00Z")
	})

	t.Run("null runs becomes empty slice", func(t *testing.T) {
		c, _ := fakeServer(t, http.StatusOK, `{"runs":null}`)

		got, err := c.ListRuns(t.Context(), ListRunsParams{})
		require.NoError(t, err)
		assert.NotNil(t, got)
		assert.Empty(t, got)
	})
}

func TestGetRun(t *testing.T) {
	t.Run("decodes detail", func(t *testing.T) {
		c, rec := fakeServer(t, http.StatusOK, `{"run":`+runJSON+`,"steps":[{"id":1,"run_id":"r1","step":"scan","status":"lossy","started_at":"2026-01-01T00:00:00Z","finished_at":"2026-01-01T00:00:01Z"}]}`)

		got, err := c.GetRun(t.Context(), "r1")
		require.NoError(t, err)
		assert.Equal(t, "r1", got.Run.ID)
		require.Len(t, got.Steps, 1)
		assert.Equal(t, runs.StepLossy, got.Steps[0].Status)
		assert.Equal(t, "/v1/runs/r1", rec.path)
	})

	t.Run("404 maps to not found", func(t *testing.T) {
		c, _ := fakeServer(t, http.StatusNotFound, `{"detail":"missing"}`)

		_, err := c.GetRun(t.Context(), "nope")
		require.ErrorIs(t, err, domain.ErrNotFound)
	})
}

func TestPruneRuns(t *testing.T) {
	t.Run("sends days and returns count", func(t *testing.T) {
		c, rec := fakeServer(t, http.StatusOK, `{"count":4}`)

		n, err := c.PruneRuns(t.Context(), 30)
		require.NoError(t, err)
		assert.Equal(t, 4, n)
		assert.Equal(t, "/v1/runs/prune", rec.path)
		assert.InDelta(t, 30, rec.body["older_than_days"], 0)
	})
}
