package synthapi

import (
	"context"
	"net/http"
	"testing"

	"claude-memory-sync/internal/domain"
	"claude-memory-sync/internal/pipeline/client"
	"claude-memory-sync/internal/runs"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListRuns(t *testing.T) {
	t.Run("passes filters and default limit", func(t *testing.T) {
		pl := &MockPipeline{ListRunsFunc: func(_ context.Context, _ client.ListRunsParams) ([]runs.Run, error) {
			return []runs.Run{{ID: "r1"}}, nil
		}}
		rec := do(t, New(Deps{Pipeline: pl}), http.MethodGet, "/api/runs?flow=intake&status=failed", "")

		require.Equal(t, http.StatusOK, rec.Code)
		params := pl.ListRunsCalls()[0].Params
		assert.Equal(t, "intake", params.Flow)
		assert.Equal(t, runs.StatusFailed, *params.Status)
		assert.Equal(t, 50, params.Limit)
		assert.Nil(t, params.Before)
	})

	t.Run("before is forwarded", func(t *testing.T) {
		pl := &MockPipeline{ListRunsFunc: func(_ context.Context, _ client.ListRunsParams) ([]runs.Run, error) { return nil, nil }}
		rec := do(t, New(Deps{Pipeline: pl}), http.MethodGet, "/api/runs?before=2026-01-01T00:00:00Z&limit=5", "")

		require.Equal(t, http.StatusOK, rec.Code)
		assert.JSONEq(t, `{"runs":[]}`, rec.Body.String())
		assert.True(t, t0.Equal(*pl.ListRunsCalls()[0].Params.Before))
		assert.Equal(t, 5, pl.ListRunsCalls()[0].Params.Limit)
	})

	t.Run("limit over 200 is 422", func(t *testing.T) {
		rec := do(t, New(Deps{Pipeline: &MockPipeline{}}), http.MethodGet, "/api/runs?limit=201", "")
		assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	})

	t.Run("invalid status is 422", func(t *testing.T) {
		rec := do(t, New(Deps{Pipeline: &MockPipeline{}}), http.MethodGet, "/api/runs?status=bogus", "")
		assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	})

	t.Run("upstream error is 502", func(t *testing.T) {
		pl := &MockPipeline{ListRunsFunc: func(_ context.Context, _ client.ListRunsParams) ([]runs.Run, error) {
			return nil, domain.Error(domain.ErrUpstream, "down")
		}}
		rec := do(t, New(Deps{Pipeline: pl}), http.MethodGet, "/api/runs", "")
		assert.Equal(t, http.StatusBadGateway, rec.Code)
	})
}

func TestGetRun(t *testing.T) {
	t.Run("returns run with empty steps list", func(t *testing.T) {
		pl := &MockPipeline{GetRunFunc: func(_ context.Context, id string) (runs.RunDetail, error) {
			return runs.RunDetail{Run: runs.Run{ID: id}}, nil
		}}
		rec := do(t, New(Deps{Pipeline: pl}), http.MethodGet, "/api/runs/r1", "")

		require.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), `"steps":[]`)
		assert.Contains(t, rec.Body.String(), `"id":"r1"`)
	})

	t.Run("not found is 404", func(t *testing.T) {
		pl := &MockPipeline{GetRunFunc: func(_ context.Context, _ string) (runs.RunDetail, error) {
			return runs.RunDetail{}, domain.Error(domain.ErrNotFound, "no run")
		}}
		rec := do(t, New(Deps{Pipeline: pl}), http.MethodGet, "/api/runs/x", "")
		assert.Equal(t, http.StatusNotFound, rec.Code)
	})
}
