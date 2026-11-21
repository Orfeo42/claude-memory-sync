package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"claude-memory-sync/internal/domain"
	"claude-memory-sync/internal/pipeline"
	pipelineapi "claude-memory-sync/internal/pipeline/httpapi"
	"claude-memory-sync/internal/runs"
	"claude-memory-sync/internal/runs/httpapi"
)

const (
	writerToken = "writer-token"
	adminToken  = "admin-token"
	runID       = "11111111-1111-1111-1111-111111111111"
)

func newHandler(repo *runs.MockRepository) http.Handler {
	return pipelineapi.New(context.Background(), pipeline.NewService(&pipeline.MockRepository{}), writerToken, adminToken,
		httpapi.Register(runs.NewService(repo)))
}

func do(t *testing.T, h http.Handler, method, path, token, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func decodeCount(t *testing.T, rec *httptest.ResponseRecorder) int {
	t.Helper()
	var got struct {
		Count int `json:"count"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	return got.Count
}

func TestAuth(t *testing.T) {
	h := newHandler(&runs.MockRepository{
		ListRunsFunc: func(_ context.Context, _ runs.ListFilter) ([]runs.Run, error) { return nil, nil },
	})

	t.Run("no token is 401", func(t *testing.T) {
		rec := do(t, h, http.MethodGet, "/v1/runs", "", "")
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	t.Run("wrong token is 401", func(t *testing.T) {
		rec := do(t, h, http.MethodGet, "/v1/runs", "nope", "")
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	t.Run("writer token is accepted", func(t *testing.T) {
		rec := do(t, h, http.MethodGet, "/v1/runs", writerToken, "")
		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("admin token is accepted on writer routes", func(t *testing.T) {
		rec := do(t, h, http.MethodGet, "/v1/runs", adminToken, "")
		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("write route without token is 401", func(t *testing.T) {
		rec := do(t, h, http.MethodPost, "/v1/runs", "", `{"flow":"intake","trigger":"manual"}`)
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})
}

func TestCreateRun(t *testing.T) {
	repo := &runs.MockRepository{
		CreateRunFunc: func(_ context.Context, n runs.NewRun) (runs.Run, error) {
			return runs.Run{ID: runID, Flow: n.Flow, Trigger: n.Trigger, Status: runs.StatusRunning, Summary: json.RawMessage(`{}`)}, nil
		},
	}
	h := newHandler(repo)

	t.Run("creates a run", func(t *testing.T) {
		rec := do(t, h, http.MethodPost, "/v1/runs", writerToken, `{"flow":"daily-synth","trigger":"schedule"}`)
		require.Equal(t, http.StatusCreated, rec.Code)

		var got runs.Run
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
		assert.Equal(t, runID, got.ID)
		assert.Equal(t, runs.StatusRunning, got.Status)
		assert.Equal(t, runs.NewRun{Flow: "daily-synth", Trigger: runs.TriggerSchedule}, repo.CreateRunCalls()[0].R)
	})

	t.Run("invalid trigger is rejected", func(t *testing.T) {
		rec := do(t, h, http.MethodPost, "/v1/runs", writerToken, `{"flow":"daily-synth","trigger":"cron"}`)
		assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	})

	t.Run("flow with uppercase is rejected", func(t *testing.T) {
		rec := do(t, h, http.MethodPost, "/v1/runs", writerToken, `{"flow":"Daily","trigger":"manual"}`)
		assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	})

	t.Run("empty flow is rejected", func(t *testing.T) {
		rec := do(t, h, http.MethodPost, "/v1/runs", writerToken, `{"flow":"","trigger":"manual"}`)
		assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	})
}

func TestAddStep(t *testing.T) {
	stepBody := `{"step":"scan","status":"succeeded","started_at":"2026-01-01T00:00:00Z","finished_at":"2026-01-01T00:00:01Z"}`

	t.Run("adds a step", func(t *testing.T) {
		repo := &runs.MockRepository{
			AddStepFunc: func(_ context.Context, s runs.NewStep) (runs.Step, error) {
				return runs.Step{ID: 1, RunID: s.RunID, Step: s.Step, Status: s.Status, StartedAt: s.StartedAt, FinishedAt: s.FinishedAt}, nil
			},
		}

		rec := do(t, newHandler(repo), http.MethodPost, "/v1/runs/"+runID+"/steps", writerToken, stepBody)
		require.Equal(t, http.StatusCreated, rec.Code)
		assert.Equal(t, runID, repo.AddStepCalls()[0].S.RunID)
		assert.Equal(t, runs.StepSucceeded, repo.AddStepCalls()[0].S.Status)
	})

	t.Run("invalid step status is rejected", func(t *testing.T) {
		body := strings.Replace(stepBody, "succeeded", "running", 1)
		rec := do(t, newHandler(&runs.MockRepository{}), http.MethodPost, "/v1/runs/"+runID+"/steps", writerToken, body)
		assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	})

	t.Run("output tail over the limit is rejected", func(t *testing.T) {
		body := `{"step":"scan","status":"failed","started_at":"2026-01-01T00:00:00Z","finished_at":"2026-01-01T00:00:01Z","output_tail":"` + strings.Repeat("x", 8001) + `"}`
		rec := do(t, newHandler(&runs.MockRepository{}), http.MethodPost, "/v1/runs/"+runID+"/steps", writerToken, body)
		assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	})

	t.Run("finish before start is 400", func(t *testing.T) {
		body := `{"step":"scan","status":"succeeded","started_at":"2026-01-01T00:00:05Z","finished_at":"2026-01-01T00:00:01Z"}`
		rec := do(t, newHandler(&runs.MockRepository{}), http.MethodPost, "/v1/runs/"+runID+"/steps", writerToken, body)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("unknown run is 404", func(t *testing.T) {
		repo := &runs.MockRepository{
			AddStepFunc: func(_ context.Context, _ runs.NewStep) (runs.Step, error) {
				return runs.Step{}, domain.Error(domain.ErrNotFound, "run not found")
			},
		}
		rec := do(t, newHandler(repo), http.MethodPost, "/v1/runs/"+runID+"/steps", writerToken, stepBody)
		assert.Equal(t, http.StatusNotFound, rec.Code)
	})

	t.Run("finished run is 409", func(t *testing.T) {
		repo := &runs.MockRepository{
			AddStepFunc: func(_ context.Context, _ runs.NewStep) (runs.Step, error) {
				return runs.Step{}, domain.Error(domain.ErrConflict, "run is not running")
			},
		}
		rec := do(t, newHandler(repo), http.MethodPost, "/v1/runs/"+runID+"/steps", writerToken, stepBody)
		assert.Equal(t, http.StatusConflict, rec.Code)
	})
}

func TestFinishRun(t *testing.T) {
	t.Run("finishes with summary", func(t *testing.T) {
		repo := &runs.MockRepository{
			FinishRunFunc: func(_ context.Context, f runs.Finish) (runs.Run, error) {
				return runs.Run{ID: f.ID, Status: f.Status, Summary: f.Summary}, nil
			},
		}

		rec := do(t, newHandler(repo), http.MethodPost, "/v1/runs/"+runID+"/finish", writerToken, `{"status":"partial","summary":{"proposals":2}}`)
		require.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, runs.StatusPartial, repo.FinishRunCalls()[0].F.Status)
		assert.JSONEq(t, `{"proposals":2}`, string(repo.FinishRunCalls()[0].F.Summary))
	})

	t.Run("running status is rejected", func(t *testing.T) {
		rec := do(t, newHandler(&runs.MockRepository{}), http.MethodPost, "/v1/runs/"+runID+"/finish", writerToken, `{"status":"running"}`)
		assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	})

	t.Run("interrupted status is rejected", func(t *testing.T) {
		rec := do(t, newHandler(&runs.MockRepository{}), http.MethodPost, "/v1/runs/"+runID+"/finish", writerToken, `{"status":"interrupted"}`)
		assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	})

	t.Run("already finished is 409", func(t *testing.T) {
		repo := &runs.MockRepository{
			FinishRunFunc: func(_ context.Context, _ runs.Finish) (runs.Run, error) {
				return runs.Run{}, domain.Error(domain.ErrConflict, "run is not running")
			},
		}
		rec := do(t, newHandler(repo), http.MethodPost, "/v1/runs/"+runID+"/finish", writerToken, `{"status":"failed"}`)
		assert.Equal(t, http.StatusConflict, rec.Code)
	})

	t.Run("unknown run is 404", func(t *testing.T) {
		repo := &runs.MockRepository{
			FinishRunFunc: func(_ context.Context, _ runs.Finish) (runs.Run, error) {
				return runs.Run{}, domain.Error(domain.ErrNotFound, "run not found")
			},
		}
		rec := do(t, newHandler(repo), http.MethodPost, "/v1/runs/"+runID+"/finish", writerToken, `{"status":"failed"}`)
		assert.Equal(t, http.StatusNotFound, rec.Code)
	})
}

func TestInterruptRuns(t *testing.T) {
	repo := &runs.MockRepository{
		InterruptRunsFunc: func(_ context.Context, _ *string) (int, error) { return 2, nil },
	}
	h := newHandler(repo)

	t.Run("flow filter is passed through", func(t *testing.T) {
		rec := do(t, h, http.MethodPost, "/v1/runs/interrupt", writerToken, `{"flow":"intake"}`)
		require.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, 2, decodeCount(t, rec))
		require.NotNil(t, repo.InterruptRunsCalls()[0].Flow)
		assert.Equal(t, "intake", *repo.InterruptRunsCalls()[0].Flow)
	})

	t.Run("empty body interrupts all flows", func(t *testing.T) {
		rec := do(t, h, http.MethodPost, "/v1/runs/interrupt", writerToken, `{}`)
		require.Equal(t, http.StatusOK, rec.Code)
		assert.Nil(t, repo.InterruptRunsCalls()[1].Flow)
	})
}

func TestListRuns(t *testing.T) {
	t.Run("returns a wrapped object and applies the default limit", func(t *testing.T) {
		repo := &runs.MockRepository{
			ListRunsFunc: func(_ context.Context, _ runs.ListFilter) ([]runs.Run, error) {
				return []runs.Run{{ID: runID, Flow: "intake", Status: runs.StatusRunning, Summary: json.RawMessage(`{}`)}}, nil
			},
		}

		rec := do(t, newHandler(repo), http.MethodGet, "/v1/runs", writerToken, "")
		require.Equal(t, http.StatusOK, rec.Code)

		var got struct {
			Runs []runs.Run `json:"runs"`
		}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
		require.Len(t, got.Runs, 1)
		assert.Equal(t, 50, repo.ListRunsCalls()[0].F.Limit)
	})

	t.Run("passes filters", func(t *testing.T) {
		repo := &runs.MockRepository{
			ListRunsFunc: func(_ context.Context, _ runs.ListFilter) ([]runs.Run, error) { return nil, nil },
		}

		rec := do(t, newHandler(repo), http.MethodGet, "/v1/runs?flow=intake&status=failed&limit=5&before=2026-01-02T00:00:00Z", writerToken, "")
		require.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), `"runs":[]`)

		f := repo.ListRunsCalls()[0].F
		require.NotNil(t, f.Flow)
		assert.Equal(t, "intake", *f.Flow)
		require.NotNil(t, f.Status)
		assert.Equal(t, runs.StatusFailed, *f.Status)
		assert.Equal(t, 5, f.Limit)
		require.NotNil(t, f.Before)
		assert.True(t, f.Before.Equal(time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)))
	})

	t.Run("invalid status is rejected", func(t *testing.T) {
		rec := do(t, newHandler(&runs.MockRepository{}), http.MethodGet, "/v1/runs?status=paused", writerToken, "")
		assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	})

	t.Run("limit over 200 is rejected", func(t *testing.T) {
		rec := do(t, newHandler(&runs.MockRepository{}), http.MethodGet, "/v1/runs?limit=201", writerToken, "")
		assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	})
}

func TestGetRun(t *testing.T) {
	t.Run("returns run and steps", func(t *testing.T) {
		repo := &runs.MockRepository{
			GetRunFunc: func(_ context.Context, id string) (runs.Run, error) {
				return runs.Run{ID: id, Summary: json.RawMessage(`{}`)}, nil
			},
			StepsFunc: func(_ context.Context, _ string) ([]runs.Step, error) { return nil, nil },
		}

		rec := do(t, newHandler(repo), http.MethodGet, "/v1/runs/"+runID, writerToken, "")
		require.Equal(t, http.StatusOK, rec.Code)

		var got runs.RunDetail
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
		assert.Equal(t, runID, got.Run.ID)
		assert.Contains(t, rec.Body.String(), `"steps":[]`)
	})

	t.Run("unknown run is 404", func(t *testing.T) {
		repo := &runs.MockRepository{
			GetRunFunc: func(_ context.Context, _ string) (runs.Run, error) {
				return runs.Run{}, domain.Error(domain.ErrNotFound, "run not found")
			},
		}
		rec := do(t, newHandler(repo), http.MethodGet, "/v1/runs/"+runID, writerToken, "")
		assert.Equal(t, http.StatusNotFound, rec.Code)
	})
}

func TestPruneRuns(t *testing.T) {
	repo := &runs.MockRepository{
		PruneRunsFunc: func(_ context.Context, _ time.Duration) (int, error) { return 4, nil },
	}
	h := newHandler(repo)

	t.Run("converts days to a duration", func(t *testing.T) {
		rec := do(t, h, http.MethodPost, "/v1/runs/prune", writerToken, `{"older_than_days":30}`)
		require.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, 4, decodeCount(t, rec))
		assert.Equal(t, 30*24*time.Hour, repo.PruneRunsCalls()[0].OlderThan)
	})

	t.Run("missing days defaults to 90", func(t *testing.T) {
		rec := do(t, h, http.MethodPost, "/v1/runs/prune", writerToken, `{}`)
		require.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, 90*24*time.Hour, repo.PruneRunsCalls()[1].OlderThan)
	})

	t.Run("zero days is rejected", func(t *testing.T) {
		rec := do(t, h, http.MethodPost, "/v1/runs/prune", writerToken, `{"older_than_days":0}`)
		assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	})
}
