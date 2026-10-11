package client

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"claude-memory-sync/internal/domain"
	"claude-memory-sync/internal/pipeline"
)

const testToken = "tok"

type recorded struct {
	method string
	path   string
	query  string
	auth   string
	body   map[string]any
}

func fakeServer(t *testing.T, status int, response string) (*Client, *recorded) {
	t.Helper()

	rec := &recorded{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.method = r.Method
		rec.path = r.URL.Path
		rec.query = r.URL.RawQuery
		rec.auth = r.Header.Get("Authorization")
		if r.ContentLength > 0 {
			assert.NoError(t, json.NewDecoder(r.Body).Decode(&rec.body))
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, err := w.Write([]byte(response))
		assert.NoError(t, err)
	}))
	t.Cleanup(srv.Close)

	return New(Config{URL: srv.URL + "/", Token: testToken}, srv.Client()), rec
}

const proposalJSON = `{"id":"p1","source_flow":"f","source_run":"r","kind":"rule","name":"x.md","status":"pending","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z","files":[{"target_path":"global/rules/x.md","content":"c"}]}`

func TestCreate(t *testing.T) {
	t.Run("posts body and returns proposal", func(t *testing.T) {
		c, rec := fakeServer(t, http.StatusCreated, proposalJSON)

		got, err := c.Create(t.Context(), CreateRequest{
			SourceFlow: "f",
			SourceRun:  "r",
			Files:      []pipeline.File{{TargetPath: "global/rules/x.md", Content: "c"}},
		})
		require.NoError(t, err)
		assert.Equal(t, "p1", got.ID)
		assert.Equal(t, http.MethodPost, rec.method)
		assert.Equal(t, "/v1/proposals", rec.path)
		assert.Equal(t, "Bearer "+testToken, rec.auth)
		assert.Equal(t, "f", rec.body["source_flow"])
		assert.Equal(t, "r", rec.body["source_run"])
	})

	t.Run("409 maps to conflict with problem detail", func(t *testing.T) {
		c, _ := fakeServer(t, http.StatusConflict, `{"detail":"open proposal exists"}`)

		_, err := c.Create(t.Context(), CreateRequest{})
		require.ErrorIs(t, err, domain.ErrConflict)

		var upstream *domain.UpstreamError
		require.ErrorAs(t, err, &upstream)
		assert.Equal(t, "open proposal exists", upstream.Body)
		assert.Equal(t, http.StatusConflict, upstream.StatusCode)
	})

	t.Run("400 maps to validation", func(t *testing.T) {
		c, _ := fakeServer(t, http.StatusBadRequest, `{"detail":"bad"}`)

		_, err := c.Create(t.Context(), CreateRequest{})
		require.ErrorIs(t, err, domain.ErrValidation)
	})

	t.Run("500 maps to upstream with raw body", func(t *testing.T) {
		c, _ := fakeServer(t, http.StatusInternalServerError, `boom`)

		_, err := c.Create(t.Context(), CreateRequest{})
		require.ErrorIs(t, err, domain.ErrUpstream)

		var upstream *domain.UpstreamError
		require.ErrorAs(t, err, &upstream)
		assert.Equal(t, "boom", upstream.Body)
	})
}

func TestList(t *testing.T) {
	t.Run("sends filters and decodes proposals", func(t *testing.T) {
		c, rec := fakeServer(t, http.StatusOK, `{"proposals":[`+proposalJSON+`]}`)
		status := pipeline.StatusPending
		kind := pipeline.KindRule
		before := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)

		got, err := c.List(t.Context(), ListParams{Status: &status, Kind: &kind, Before: &before, Limit: 5})
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Equal(t, http.MethodGet, rec.method)
		assert.Equal(t, "/v1/proposals", rec.path)
		assert.Contains(t, rec.query, "status=pending")
		assert.Contains(t, rec.query, "kind=rule")
		assert.Contains(t, rec.query, "limit=5")
		assert.Contains(t, rec.query, "before=2026-01-02T00%3A00%3A00Z")
		assert.Equal(t, "Bearer "+testToken, rec.auth)
	})

	t.Run("null proposals becomes empty slice", func(t *testing.T) {
		c, _ := fakeServer(t, http.StatusOK, `{"proposals":null}`)

		got, err := c.List(t.Context(), ListParams{})
		require.NoError(t, err)
		assert.NotNil(t, got)
		assert.Empty(t, got)
	})
}

func TestGet(t *testing.T) {
	t.Run("decodes detail", func(t *testing.T) {
		c, rec := fakeServer(t, http.StatusOK, `{"proposal":`+proposalJSON+`,"events":[{"id":1,"proposal_id":"p1","to_status":"pending","actor":"a","at":"2026-01-01T00:00:00Z"}]}`)

		got, err := c.Get(t.Context(), "p1")
		require.NoError(t, err)
		assert.Equal(t, "p1", got.Proposal.ID)
		require.Len(t, got.Events, 1)
		assert.Equal(t, "/v1/proposals/p1", rec.path)
	})

	t.Run("404 maps to not found", func(t *testing.T) {
		c, _ := fakeServer(t, http.StatusNotFound, `{"detail":"missing"}`)

		_, err := c.Get(t.Context(), "nope")
		require.ErrorIs(t, err, domain.ErrNotFound)
	})
}

func TestDecide(t *testing.T) {
	t.Run("posts decision body", func(t *testing.T) {
		c, rec := fakeServer(t, http.StatusOK, proposalJSON)

		_, err := c.Decide(t.Context(), "p1", DecideRequest{Decision: "reject", Actor: "me", Reason: "dup"})
		require.NoError(t, err)
		assert.Equal(t, http.MethodPost, rec.method)
		assert.Equal(t, "/v1/proposals/p1/decision", rec.path)
		assert.Equal(t, "reject", rec.body["decision"])
		assert.Equal(t, "me", rec.body["actor"])
		assert.Equal(t, "dup", rec.body["reason"])
	})

	t.Run("omits empty reason", func(t *testing.T) {
		c, rec := fakeServer(t, http.StatusOK, proposalJSON)

		_, err := c.Decide(t.Context(), "p1", DecideRequest{Decision: "approve", Actor: "me"})
		require.NoError(t, err)
		assert.NotContains(t, rec.body, "reason")
	})

	t.Run("409 maps to conflict", func(t *testing.T) {
		c, _ := fakeServer(t, http.StatusConflict, `{"detail":"not pending"}`)

		_, err := c.Decide(t.Context(), "p1", DecideRequest{})
		require.ErrorIs(t, err, domain.ErrConflict)
	})
}

func TestRevise(t *testing.T) {
	t.Run("puts files", func(t *testing.T) {
		c, rec := fakeServer(t, http.StatusOK, proposalJSON)

		_, err := c.Revise(t.Context(), "p1", ReviseRequest{
			Actor: "me",
			Files: []pipeline.File{{TargetPath: "global/rules/x.md", Content: "new"}},
		})
		require.NoError(t, err)
		assert.Equal(t, http.MethodPut, rec.method)
		assert.Equal(t, "/v1/proposals/p1/files", rec.path)
		assert.Equal(t, "me", rec.body["actor"])
		assert.Len(t, rec.body["files"], 1)
	})
}

func TestRetry(t *testing.T) {
	t.Run("posts actor", func(t *testing.T) {
		c, rec := fakeServer(t, http.StatusOK, proposalJSON)

		_, err := c.Retry(t.Context(), "p1", "me")
		require.NoError(t, err)
		assert.Equal(t, "/v1/proposals/p1/retry", rec.path)
		assert.Equal(t, "me", rec.body["actor"])
	})
}

func TestClaim(t *testing.T) {
	t.Run("200 returns proposal", func(t *testing.T) {
		c, rec := fakeServer(t, http.StatusOK, proposalJSON)

		got, err := c.Claim(t.Context(), "w1", 60)
		require.NoError(t, err)
		assert.Equal(t, "p1", got.ID)
		assert.Equal(t, "/v1/proposals/claim", rec.path)
		assert.Equal(t, "w1", rec.body["worker"])
		assert.InDelta(t, 60, rec.body["lease_seconds"], 0)
	})

	t.Run("204 returns nothing to claim", func(t *testing.T) {
		c, _ := fakeServer(t, http.StatusNoContent, "")

		got, err := c.Claim(t.Context(), "w1", 0)
		require.ErrorIs(t, err, pipeline.ErrNothingToClaim)
		assert.Equal(t, pipeline.Proposal{}, got)
	})

	t.Run("zero lease is omitted", func(t *testing.T) {
		c, rec := fakeServer(t, http.StatusNoContent, "")

		_, err := c.Claim(t.Context(), "w1", 0)
		require.ErrorIs(t, err, pipeline.ErrNothingToClaim)
		assert.NotContains(t, rec.body, "lease_seconds")
	})
}

func TestMarkApplied(t *testing.T) {
	t.Run("posts sha", func(t *testing.T) {
		c, rec := fakeServer(t, http.StatusOK, proposalJSON)

		_, err := c.MarkApplied(t.Context(), "p1", "w1", "abc")
		require.NoError(t, err)
		assert.Equal(t, "/v1/proposals/p1/applied", rec.path)
		assert.Equal(t, "w1", rec.body["actor"])
		assert.Equal(t, "abc", rec.body["sha"])
	})

	t.Run("409 maps to conflict", func(t *testing.T) {
		c, _ := fakeServer(t, http.StatusConflict, `{"detail":"not applying"}`)

		_, err := c.MarkApplied(t.Context(), "p1", "w1", "abc")
		require.ErrorIs(t, err, domain.ErrConflict)
	})
}

func TestMarkFailed(t *testing.T) {
	t.Run("posts error text", func(t *testing.T) {
		c, rec := fakeServer(t, http.StatusOK, proposalJSON)

		_, err := c.MarkFailed(t.Context(), "p1", "w1", "boom")
		require.NoError(t, err)
		assert.Equal(t, "/v1/proposals/p1/failed", rec.path)
		assert.Equal(t, "boom", rec.body["error"])
	})
}

func TestExpire(t *testing.T) {
	t.Run("returns expired count", func(t *testing.T) {
		c, rec := fakeServer(t, http.StatusOK, `{"expired":3}`)

		got, err := c.Expire(t.Context(), "cron", 336)
		require.NoError(t, err)
		assert.Equal(t, 3, got)
		assert.Equal(t, "/v1/proposals/expire", rec.path)
		assert.Equal(t, "cron", rec.body["actor"])
		assert.InDelta(t, 336, rec.body["older_than_hours"], 0)
	})
}

func TestRejections(t *testing.T) {
	t.Run("sends kind filter", func(t *testing.T) {
		c, rec := fakeServer(t, http.StatusOK, `{"rejections":[{"kind":"skill","name":"s","rejected_at":"2026-01-01T00:00:00Z"}]}`)
		kind := pipeline.KindSkill

		got, err := c.Rejections(t.Context(), &kind)
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Equal(t, "/v1/rejections", rec.path)
		assert.Equal(t, "kind=skill", rec.query)
	})

	t.Run("no kind sends no query", func(t *testing.T) {
		c, rec := fakeServer(t, http.StatusOK, `{"rejections":null}`)

		got, err := c.Rejections(t.Context(), nil)
		require.NoError(t, err)
		assert.NotNil(t, got)
		assert.Empty(t, rec.query)
	})
}

func TestAddRejection(t *testing.T) {
	t.Run("posts body with auth and accepts 204", func(t *testing.T) {
		c, rec := fakeServer(t, http.StatusNoContent, "")
		reason := "why"

		err := c.AddRejection(t.Context(), AddRejectionRequest{Kind: pipeline.KindSkill, Name: "s", Actor: "ui", Reason: &reason})
		require.NoError(t, err)
		assert.Equal(t, http.MethodPost, rec.method)
		assert.Equal(t, "/v1/rejections", rec.path)
		assert.Equal(t, "Bearer "+testToken, rec.auth)
		assert.Equal(t, "skill", rec.body["kind"])
		assert.Equal(t, "s", rec.body["name"])
		assert.Equal(t, "ui", rec.body["actor"])
		assert.Equal(t, "why", rec.body["reason"])
	})

	t.Run("nil reason is omitted", func(t *testing.T) {
		c, rec := fakeServer(t, http.StatusNoContent, "")

		err := c.AddRejection(t.Context(), AddRejectionRequest{Kind: pipeline.KindSkill, Name: "s", Actor: "ui"})
		require.NoError(t, err)
		assert.NotContains(t, rec.body, "reason")
	})

	t.Run("403 maps to upstream error with status and detail", func(t *testing.T) {
		c, _ := fakeServer(t, http.StatusForbidden, `{"detail":"admin only"}`)

		err := c.AddRejection(t.Context(), AddRejectionRequest{Kind: pipeline.KindSkill, Name: "s", Actor: "ui"})
		var upstream *domain.UpstreamError
		require.ErrorAs(t, err, &upstream)
		assert.Equal(t, http.StatusForbidden, upstream.StatusCode)
		assert.Equal(t, "admin only", upstream.Body)
	})

	t.Run("422 maps to validation", func(t *testing.T) {
		c, _ := fakeServer(t, http.StatusUnprocessableEntity, `{"detail":"bad kind"}`)

		err := c.AddRejection(t.Context(), AddRejectionRequest{Kind: pipeline.KindSkill, Name: "s", Actor: "ui"})
		require.ErrorIs(t, err, domain.ErrValidation)
	})
}

func TestUnreachable(t *testing.T) {
	t.Run("closed server maps to unreachable error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
		c := New(Config{URL: srv.URL, Token: testToken}, srv.Client())
		srv.Close()

		_, err := c.Get(t.Context(), "p1")
		require.ErrorIs(t, err, domain.ErrUpstream)

		var unreachable *domain.UpstreamUnreachableError
		require.ErrorAs(t, err, &unreachable)
		assert.Equal(t, "pipeline", unreachable.API)
	})
}
