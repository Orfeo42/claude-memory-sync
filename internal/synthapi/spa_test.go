package synthapi

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func webFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	web := filepath.Join(root, "web")
	require.NoError(t, os.MkdirAll(filepath.Join(web, "assets"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(web, "index.html"), []byte("<html>spa</html>"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(web, "assets", "app.js"), []byte("console.log(1)"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(root, "go.mod"), []byte("SECRET-OUTSIDE"), 0o600))
	return web
}

func TestSPAHandler(t *testing.T) {
	t.Run("existing file is served", func(t *testing.T) {
		h := New(Deps{WebDir: webFixture(t)})
		rec := do(t, h, http.MethodGet, "/assets/app.js", "")
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "console.log(1)", rec.Body.String())
	})

	t.Run("unknown path falls back to index", func(t *testing.T) {
		h := New(Deps{WebDir: webFixture(t)})
		rec := do(t, h, http.MethodGet, "/proposals/42", "")
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "<html>spa</html>", rec.Body.String())
	})

	t.Run("root serves index", func(t *testing.T) {
		h := New(Deps{WebDir: webFixture(t)})
		rec := do(t, h, http.MethodGet, "/", "")
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "<html>spa</html>", rec.Body.String())
	})

	t.Run("unknown api path is 404 not index", func(t *testing.T) {
		h := New(Deps{WebDir: webFixture(t)})
		rec := do(t, h, http.MethodGet, "/api/unknown", "")
		assert.Equal(t, http.StatusNotFound, rec.Code)
		assert.NotContains(t, rec.Body.String(), "spa")
	})

	t.Run("path traversal does not leak outside files", func(t *testing.T) {
		h := New(Deps{WebDir: webFixture(t)})
		rec := do(t, h, http.MethodGet, "/../go.mod", "")
		assert.NotContains(t, rec.Body.String(), "SECRET-OUTSIDE")
		rec = do(t, h, http.MethodGet, "/..%2fgo.mod", "")
		assert.NotContains(t, rec.Body.String(), "SECRET-OUTSIDE")
		rec = do(t, h, http.MethodGet, "/assets/../../go.mod", "")
		assert.NotContains(t, rec.Body.String(), "SECRET-OUTSIDE")
	})

	t.Run("missing web dir gives 404 for spa paths", func(t *testing.T) {
		h := New(Deps{WebDir: filepath.Join(t.TempDir(), "absent")})
		rec := do(t, h, http.MethodGet, "/anything", "")
		assert.Equal(t, http.StatusNotFound, rec.Code)
	})

	t.Run("empty web dir gives 404 for spa paths", func(t *testing.T) {
		h := New(Deps{})
		rec := do(t, h, http.MethodGet, "/", "")
		assert.Equal(t, http.StatusNotFound, rec.Code)
	})
}
