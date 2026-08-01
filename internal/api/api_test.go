package api_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"claude-memory-sync/internal/api"

	"github.com/stretchr/testify/assert"
)

const testToken = "test-token"

var stubGitHandler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
})

func doRequest(t *testing.T, handler http.Handler, path, token string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, strings.NewReader(""))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestHealthz(t *testing.T) {
	t.Run("requires no auth", func(t *testing.T) {
		handler := api.New(testToken, stubGitHandler)

		rec := doRequest(t, handler, "/v1/healthz", "")
		assert.Equal(t, http.StatusOK, rec.Code)
	})
}

func TestGitRouteMounting(t *testing.T) {
	t.Run("rejects missing token", func(t *testing.T) {
		handler := api.New(testToken, stubGitHandler)

		rec := doRequest(t, handler, "/git/canonical.git/info/refs", "")
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	t.Run("rejects wrong token", func(t *testing.T) {
		handler := api.New(testToken, stubGitHandler)

		rec := doRequest(t, handler, "/git/canonical.git/info/refs", "wrong")
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	t.Run("delegates to git handler with valid token", func(t *testing.T) {
		handler := api.New(testToken, stubGitHandler)

		rec := doRequest(t, handler, "/git/canonical.git/info/refs", testToken)
		assert.Equal(t, http.StatusOK, rec.Code)
	})
}
