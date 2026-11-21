package synthapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"claude-memory-sync/internal/domain"
	"claude-memory-sync/internal/pipeline"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func do(t *testing.T, h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func testProposal(id string, status pipeline.Status, created time.Time) pipeline.Proposal {
	return pipeline.Proposal{
		ID:        id,
		Status:    status,
		CreatedAt: created,
		Files:     []pipeline.File{{TargetPath: "global/skills/a/SKILL.md", Content: "old\n"}},
	}
}

func TestToHumaError(t *testing.T) {
	t.Run("validation maps to 422", func(t *testing.T) {
		err := toHumaError(context.Background(), domain.Error(domain.ErrValidation, "bad"))
		assert.Contains(t, err.Error(), "bad")
		var status interface{ GetStatus() int }
		require.ErrorAs(t, err, &status)
		assert.Equal(t, http.StatusUnprocessableEntity, status.GetStatus())
	})

	t.Run("upstream maps to 502", func(t *testing.T) {
		err := toHumaError(context.Background(), domain.Error(domain.ErrUpstream, "down"))
		var status interface{ GetStatus() int }
		require.ErrorAs(t, err, &status)
		assert.Equal(t, http.StatusBadGateway, status.GetStatus())
	})

	t.Run("unknown error maps to 500", func(t *testing.T) {
		err := toHumaError(context.Background(), assert.AnError)
		var status interface{ GetStatus() int }
		require.ErrorAs(t, err, &status)
		assert.Equal(t, http.StatusInternalServerError, status.GetStatus())
	})
}
