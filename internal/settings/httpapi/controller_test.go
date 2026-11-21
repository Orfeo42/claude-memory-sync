package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"claude-memory-sync/internal/domain"
	"claude-memory-sync/internal/pipeline"
	pipelinehttpapi "claude-memory-sync/internal/pipeline/httpapi"
	"claude-memory-sync/internal/settings"
	"claude-memory-sync/internal/settings/httpapi"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	writerToken = "writer-token"
	adminToken  = "admin-token"
)

func strPtr(v string) *string {
	return &v
}

func newHandler(repo *settings.MockRepository) http.Handler {
	return pipelinehttpapi.New(context.Background(), pipeline.NewService(&pipeline.MockRepository{}), writerToken, adminToken,
		httpapi.Register(settings.NewService(repo)))
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

func echoRepo() *settings.MockRepository {
	return &settings.MockRepository{
		PutFunc: func(_ context.Context, req settings.PutRequest) (settings.Setting, error) {
			return settings.Setting{Key: string(req.Key), Value: strPtr(req.Value), Set: true, UpdatedBy: req.Actor}, nil
		},
		DeleteFunc: func(_ context.Context, _ settings.DeleteRequest) error { return nil },
	}
}

func TestAuth(t *testing.T) {
	h := newHandler(echoRepo())
	body := `{"value":"v","actor":"me"}`

	t.Run("read without token is 401", func(t *testing.T) {
		assert.Equal(t, http.StatusUnauthorized, do(t, h, http.MethodGet, "/v1/settings", "", "").Code)
	})

	t.Run("writer can read", func(t *testing.T) {
		repo := echoRepo()
		repo.ListFunc = func(_ context.Context) ([]settings.Setting, error) { return nil, nil }
		rec := do(t, newHandler(repo), http.MethodGet, "/v1/settings", writerToken, "")
		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("writer cannot put", func(t *testing.T) {
		assert.Equal(t, http.StatusForbidden, do(t, h, http.MethodPut, "/v1/settings/param.a", writerToken, body).Code)
	})

	t.Run("writer cannot delete", func(t *testing.T) {
		assert.Equal(t, http.StatusForbidden, do(t, h, http.MethodDelete, "/v1/settings/param.a?actor=me", writerToken, "").Code)
	})

	t.Run("admin can put", func(t *testing.T) {
		assert.Equal(t, http.StatusOK, do(t, h, http.MethodPut, "/v1/settings/param.a", adminToken, body).Code)
	})
}

func TestList(t *testing.T) {
	t.Run("omits secret value and wraps rows", func(t *testing.T) {
		repo := &settings.MockRepository{ListFunc: func(_ context.Context) ([]settings.Setting, error) {
			return []settings.Setting{
				{Key: "param.a", Value: strPtr("1"), Set: true, UpdatedAt: time.Unix(0, 0).UTC(), UpdatedBy: "me"},
				{Key: "secret.b", Value: strPtr("hush"), Set: true, UpdatedAt: time.Unix(0, 0).UTC(), UpdatedBy: "me"},
			}, nil
		}}

		rec := do(t, newHandler(repo), http.MethodGet, "/v1/settings", writerToken, "")
		require.Equal(t, http.StatusOK, rec.Code)
		assert.NotContains(t, rec.Body.String(), "hush")
		var resp struct {
			Settings []map[string]any `json:"settings"`
		}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		require.Len(t, resp.Settings, 2)
		assert.Equal(t, "1", resp.Settings[0]["value"])
		assert.NotContains(t, resp.Settings[1], "value")
		assert.Equal(t, true, resp.Settings[1]["set"])
	})

	t.Run("empty list is an empty array", func(t *testing.T) {
		repo := &settings.MockRepository{ListFunc: func(_ context.Context) ([]settings.Setting, error) { return nil, nil }}
		rec := do(t, newHandler(repo), http.MethodGet, "/v1/settings", writerToken, "")
		assert.Contains(t, rec.Body.String(), `"settings":[]`)
	})
}

func TestGet(t *testing.T) {
	t.Run("returns the secret value in full", func(t *testing.T) {
		repo := &settings.MockRepository{GetFunc: func(_ context.Context, key settings.Key) (settings.Setting, error) {
			return settings.Setting{Key: string(key), Value: strPtr("hush"), Set: true}, nil
		}}
		rec := do(t, newHandler(repo), http.MethodGet, "/v1/settings/secret.b", writerToken, "")
		require.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), "hush")
	})

	t.Run("unset key is 404", func(t *testing.T) {
		repo := &settings.MockRepository{GetFunc: func(_ context.Context, _ settings.Key) (settings.Setting, error) {
			return settings.Setting{}, domain.Error(domain.ErrNotFound, "setting not found")
		}}
		rec := do(t, newHandler(repo), http.MethodGet, "/v1/settings/param.a", writerToken, "")
		assert.Equal(t, http.StatusNotFound, rec.Code)
	})

	t.Run("bad key is rejected before the repository", func(t *testing.T) {
		repo := &settings.MockRepository{}
		rec := do(t, newHandler(repo), http.MethodGet, "/v1/settings/Bad_Key", writerToken, "")
		assert.GreaterOrEqual(t, rec.Code, 400)
		assert.Less(t, rec.Code, 500)
		assert.Empty(t, repo.GetCalls())
	})
}

func TestPut(t *testing.T) {
	t.Run("returns the row", func(t *testing.T) {
		repo := echoRepo()
		rec := do(t, newHandler(repo), http.MethodPut, "/v1/settings/param.a", adminToken, `{"value":"v","actor":"me"}`)
		require.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), `"value":"v"`)
		assert.Equal(t, settings.PutRequest{Key: "param.a", Value: "v", Actor: "me"}, repo.PutCalls()[0].Req)
	})

	t.Run("omits secret value in the response", func(t *testing.T) {
		rec := do(t, newHandler(echoRepo()), http.MethodPut, "/v1/settings/secret.a", adminToken, `{"value":"hush","actor":"me"}`)
		require.Equal(t, http.StatusOK, rec.Code)
		assert.NotContains(t, rec.Body.String(), "hush")
	})

	t.Run("bad key is 4xx", func(t *testing.T) {
		repo := echoRepo()
		rec := do(t, newHandler(repo), http.MethodPut, "/v1/settings/other.a", adminToken, `{"value":"v","actor":"me"}`)
		assert.GreaterOrEqual(t, rec.Code, 400)
		assert.Less(t, rec.Code, 500)
		assert.Empty(t, repo.PutCalls())
	})

	t.Run("empty actor is 4xx", func(t *testing.T) {
		rec := do(t, newHandler(echoRepo()), http.MethodPut, "/v1/settings/param.a", adminToken, `{"value":"v","actor":""}`)
		assert.GreaterOrEqual(t, rec.Code, 400)
		assert.Less(t, rec.Code, 500)
	})

	t.Run("value over the limit is 4xx", func(t *testing.T) {
		body := `{"value":"` + strings.Repeat("a", 200001) + `","actor":"me"}`
		rec := do(t, newHandler(echoRepo()), http.MethodPut, "/v1/settings/param.a", adminToken, body)
		assert.GreaterOrEqual(t, rec.Code, 400)
		assert.Less(t, rec.Code, 500)
	})
}

func TestDelete(t *testing.T) {
	t.Run("returns 204", func(t *testing.T) {
		repo := echoRepo()
		rec := do(t, newHandler(repo), http.MethodDelete, "/v1/settings/param.a?actor=me", adminToken, "")
		assert.Equal(t, http.StatusNoContent, rec.Code)
		assert.Equal(t, settings.DeleteRequest{Key: "param.a", Actor: "me"}, repo.DeleteCalls()[0].Req)
	})

	t.Run("missing actor is 4xx", func(t *testing.T) {
		rec := do(t, newHandler(echoRepo()), http.MethodDelete, "/v1/settings/param.a", adminToken, "")
		assert.GreaterOrEqual(t, rec.Code, 400)
		assert.Less(t, rec.Code, 500)
	})
}

func TestHistory(t *testing.T) {
	t.Run("returns versions with the default limit", func(t *testing.T) {
		repo := &settings.MockRepository{HistoryFunc: func(_ context.Context, _ settings.Key, _ int) ([]settings.Version, error) {
			return []settings.Version{{Value: strPtr("v"), CreatedBy: "me"}, {Deleted: true, CreatedBy: "me"}}, nil
		}}
		rec := do(t, newHandler(repo), http.MethodGet, "/v1/settings/param.a/history", writerToken, "")
		require.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, 50, repo.HistoryCalls()[0].Limit)
		var resp struct {
			Versions []map[string]any `json:"versions"`
		}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		require.Len(t, resp.Versions, 2)
		assert.Equal(t, true, resp.Versions[1]["deleted"])
	})

	t.Run("limit above 200 is 4xx", func(t *testing.T) {
		repo := &settings.MockRepository{}
		rec := do(t, newHandler(repo), http.MethodGet, "/v1/settings/param.a/history?limit=201", writerToken, "")
		assert.GreaterOrEqual(t, rec.Code, 400)
		assert.Less(t, rec.Code, 500)
		assert.Empty(t, repo.HistoryCalls())
	})

	t.Run("empty history is an empty array", func(t *testing.T) {
		repo := &settings.MockRepository{HistoryFunc: func(_ context.Context, _ settings.Key, _ int) ([]settings.Version, error) {
			return nil, nil
		}}
		rec := do(t, newHandler(repo), http.MethodGet, "/v1/settings/param.a/history", writerToken, "")
		assert.Contains(t, rec.Body.String(), `"versions":[]`)
	})
}
