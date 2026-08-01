package gitserver

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func execGit(t *testing.T, dir string, args ...string) {
	t.Helper()

	cmd := exec.Command("git", args...)
	cmd.Dir = dir

	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v: %s", args, out)
}

func alwaysSucceedCommand(t *testing.T) string {
	t.Helper()

	path, err := exec.LookPath("true")
	require.NoError(t, err)

	return path
}

func newTestServer(t *testing.T, reposRoot string) *httptest.Server {
	t.Helper()

	h := newHandler(reposRoot, alwaysSucceedCommand(t))
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	return srv
}

func newCommittedWorkTree(t *testing.T, relPath, content string) string {
	t.Helper()

	dir := t.TempDir()
	execGit(t, dir, "init", "--initial-branch=main")
	execGit(t, dir, "config", "user.email", "test@example.com")
	execGit(t, dir, "config", "user.name", "Test User")

	fullPath := filepath.Join(dir, relPath)
	require.NoError(t, os.MkdirAll(filepath.Dir(fullPath), 0o755))
	require.NoError(t, os.WriteFile(fullPath, []byte(content), 0o644))
	execGit(t, dir, "add", relPath)
	execGit(t, dir, "commit", "-m", "test commit")

	return dir
}

func TestEnsureCanonical(t *testing.T) {
	t.Run("creates canonical repo when missing", func(t *testing.T) {
		reposRoot := t.TempDir()

		err := EnsureCanonical(t.Context(), reposRoot)
		require.NoError(t, err)
		assert.FileExists(t, filepath.Join(reposRoot, "canonical.git", "HEAD"))
	})

	t.Run("idempotent when already exists", func(t *testing.T) {
		reposRoot := t.TempDir()

		require.NoError(t, EnsureCanonical(t.Context(), reposRoot))
		require.NoError(t, EnsureCanonical(t.Context(), reposRoot))
		assert.FileExists(t, filepath.Join(reposRoot, "canonical.git", "HEAD"))
	})
}

func TestServeHTTP(t *testing.T) {
	t.Run("push to a new client repo auto-creates it with an executable pre-receive hook", func(t *testing.T) {
		reposRoot := t.TempDir()
		srv := newTestServer(t, reposRoot)
		workDir := newCommittedWorkTree(t, "global/CLAUDE.md", "hello")

		execGit(t, workDir, "push", srv.URL+"/git/clients/test-1.git", "main")

		hookPath := filepath.Join(reposRoot, "clients", "test-1.git", "hooks", "pre-receive")
		info, err := os.Stat(hookPath)
		require.NoError(t, err)
		assert.NotZero(t, info.Mode().Perm()&0o100, "hook file should be executable")
	})

	t.Run("push then clone round trip returns the pushed content", func(t *testing.T) {
		reposRoot := t.TempDir()
		srv := newTestServer(t, reposRoot)
		workDir := newCommittedWorkTree(t, "global/CLAUDE.md", "round trip content")

		repoURL := srv.URL + "/git/clients/test-2.git"
		execGit(t, workDir, "push", repoURL, "main")

		cloneParent := t.TempDir()
		execGit(t, cloneParent, "clone", repoURL, "clone")

		content, err := os.ReadFile(filepath.Join(cloneParent, "clone", "global", "CLAUDE.md"))
		require.NoError(t, err)
		assert.Equal(t, "round trip content", string(content))
	})

	t.Run("push to canonical repository is forbidden", func(t *testing.T) {
		reposRoot := t.TempDir()
		srv := newTestServer(t, reposRoot)

		resp, err := http.Get(srv.URL + "/git/canonical.git/info/refs?service=git-receive-pack")
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	})

	t.Run("fetch of missing client repository returns 404", func(t *testing.T) {
		reposRoot := t.TempDir()
		srv := newTestServer(t, reposRoot)

		resp, err := http.Get(srv.URL + "/git/clients/does-not-exist.git/info/refs?service=git-upload-pack")
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

	t.Run("clone of canonical repository succeeds after EnsureCanonical", func(t *testing.T) {
		reposRoot := t.TempDir()
		require.NoError(t, EnsureCanonical(t.Context(), reposRoot))
		srv := newTestServer(t, reposRoot)

		cloneParent := t.TempDir()
		execGit(t, cloneParent, "clone", srv.URL+"/git/canonical.git", "clone")

		assert.DirExists(t, filepath.Join(cloneParent, "clone"))
	})
}
