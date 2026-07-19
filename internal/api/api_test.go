package api_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"claude-memory-sync/internal/api"
	"claude-memory-sync/internal/manifest"
	"claude-memory-sync/internal/store"
)

const testToken = "test-token"

func doRequest(t *testing.T, handler http.Handler, method, path, token, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestHealthz(t *testing.T) {
	t.Run("requires no auth", func(t *testing.T) {
		mock := &store.MockStore{}
		handler := api.New(mock, testToken)

		rec := doRequest(t, handler, http.MethodGet, "/v1/healthz", "", "")
		assert.Equal(t, http.StatusOK, rec.Code)
	})
}

func TestClientTree(t *testing.T) {
	t.Run("rejects missing token", func(t *testing.T) {
		mock := &store.MockStore{}
		handler := api.New(mock, testToken)

		rec := doRequest(t, handler, http.MethodGet, "/v1/clients/host-a/tree", "", "")
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
		assert.Contains(t, rec.Body.String(), "unauthorized")
	})

	t.Run("rejects wrong token", func(t *testing.T) {
		mock := &store.MockStore{}
		handler := api.New(mock, testToken)

		rec := doRequest(t, handler, http.MethodGet, "/v1/clients/host-a/tree", "wrong", "")
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	t.Run("returns manifest json", func(t *testing.T) {
		mock := &store.MockStore{
			TreeFunc: func(namespace string) (manifest.Manifest, error) {
				require.Equal(t, "clients/host-a", namespace)
				return manifest.Manifest{{Path: "global/CLAUDE.md", SHA256: "abc", Size: 3}}, nil
			},
		}
		handler := api.New(mock, testToken)

		rec := doRequest(t, handler, http.MethodGet, "/v1/clients/host-a/tree", testToken, "")
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), "global/CLAUDE.md")
	})

	t.Run("rejects invalid client id", func(t *testing.T) {
		mock := &store.MockStore{}
		handler := api.New(mock, testToken)

		rec := doRequest(t, handler, http.MethodGet, "/v1/clients/bad%2Fid/tree", testToken, "")
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("maps internal store error to 500", func(t *testing.T) {
		mock := &store.MockStore{
			TreeFunc: func(namespace string) (manifest.Manifest, error) {
				return nil, errors.New("boom")
			},
		}
		handler := api.New(mock, testToken)

		rec := doRequest(t, handler, http.MethodGet, "/v1/clients/host-a/tree", testToken, "")
		assert.Equal(t, http.StatusInternalServerError, rec.Code)
	})
}

func TestClientFileGet(t *testing.T) {
	t.Run("returns content and content type", func(t *testing.T) {
		mock := &store.MockStore{
			ReadFunc: func(namespace, path string) ([]byte, error) {
				require.Equal(t, "clients/host-a", namespace)
				require.Equal(t, "global/CLAUDE.md", path)
				return []byte("hello"), nil
			},
		}
		handler := api.New(mock, testToken)

		rec := doRequest(t, handler, http.MethodGet, "/v1/clients/host-a/file/global/CLAUDE.md", testToken, "")
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "hello", rec.Body.String())
		assert.Equal(t, "application/octet-stream", rec.Header().Get("Content-Type"))
	})

	t.Run("returns 404 when missing", func(t *testing.T) {
		mock := &store.MockStore{
			ReadFunc: func(namespace, path string) ([]byte, error) {
				return nil, store.ErrNotFound
			},
		}
		handler := api.New(mock, testToken)

		rec := doRequest(t, handler, http.MethodGet, "/v1/clients/host-a/file/missing.md", testToken, "")
		assert.Equal(t, http.StatusNotFound, rec.Code)
	})

	t.Run("returns 400 on invalid path", func(t *testing.T) {
		mock := &store.MockStore{
			ReadFunc: func(namespace, path string) ([]byte, error) {
				return nil, store.ErrInvalidPath
			},
		}
		handler := api.New(mock, testToken)

		rec := doRequest(t, handler, http.MethodGet, "/v1/clients/host-a/file/x", testToken, "")
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})
}

func TestClientFilePut(t *testing.T) {
	t.Run("writes content", func(t *testing.T) {
		var gotNamespace, gotPath, gotClientID string
		var gotContent []byte
		mock := &store.MockStore{
			WriteFunc: func(namespace, path string, content []byte, clientID string) error {
				gotNamespace, gotPath, gotContent, gotClientID = namespace, path, content, clientID
				return nil
			},
		}
		handler := api.New(mock, testToken)

		rec := doRequest(t, handler, http.MethodPut, "/v1/clients/host-a/file/global/CLAUDE.md", testToken, "hello")
		assert.Equal(t, http.StatusNoContent, rec.Code)
		assert.Equal(t, "clients/host-a", gotNamespace)
		assert.Equal(t, "global/CLAUDE.md", gotPath)
		assert.Equal(t, "hello", string(gotContent))
		assert.Equal(t, "host-a", gotClientID)
	})

	t.Run("rejects invalid client id", func(t *testing.T) {
		mock := &store.MockStore{}
		handler := api.New(mock, testToken)

		rec := doRequest(t, handler, http.MethodPut, "/v1/clients/bad!id/file/x", testToken, "hello")
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})
}

func TestClientFileDelete(t *testing.T) {
	t.Run("removes file", func(t *testing.T) {
		var gotNamespace, gotPath, gotClientID string
		mock := &store.MockStore{
			DeleteFunc: func(namespace, path, clientID string) error {
				gotNamespace, gotPath, gotClientID = namespace, path, clientID
				return nil
			},
		}
		handler := api.New(mock, testToken)

		rec := doRequest(t, handler, http.MethodDelete, "/v1/clients/host-a/file/global/CLAUDE.md", testToken, "")
		assert.Equal(t, http.StatusNoContent, rec.Code)
		assert.Equal(t, "clients/host-a", gotNamespace)
		assert.Equal(t, "global/CLAUDE.md", gotPath)
		assert.Equal(t, "host-a", gotClientID)
	})
}

func TestCanonicalTree(t *testing.T) {
	t.Run("returns manifest json with namespace canonical", func(t *testing.T) {
		mock := &store.MockStore{
			TreeFunc: func(namespace string) (manifest.Manifest, error) {
				require.Equal(t, "canonical", namespace)
				return manifest.Manifest{{Path: "global/CLAUDE.md", SHA256: "abc", Size: 3}}, nil
			},
		}
		handler := api.New(mock, testToken)

		rec := doRequest(t, handler, http.MethodGet, "/v1/canonical/tree", testToken, "")
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), "global/CLAUDE.md")
	})
}

func TestCanonicalFileGet(t *testing.T) {
	t.Run("returns content with namespace canonical", func(t *testing.T) {
		mock := &store.MockStore{
			ReadFunc: func(namespace, path string) ([]byte, error) {
				require.Equal(t, "canonical", namespace)
				require.Equal(t, "global/CLAUDE.md", path)
				return []byte("hello"), nil
			},
		}
		handler := api.New(mock, testToken)

		rec := doRequest(t, handler, http.MethodGet, "/v1/canonical/file/global/CLAUDE.md", testToken, "")
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "hello", rec.Body.String())
	})
}
