package syncer

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"claude-memory-sync/internal/manifest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnsureStagingRepo(t *testing.T) {
	t.Run("initializes a new staging repo with remote and auth header", func(t *testing.T) {
		cfg, _ := newTestConfig(t, "host-a", "-home-testhost")

		require.NoError(t, ensureStagingRepo(context.Background(), cfg))

		dir := stagingDir(cfg)
		assert.DirExists(t, filepath.Join(dir, ".git"))

		remote := runGit(t, dir, "remote", "get-url", "origin")
		assert.Equal(t, clientRemoteURL(cfg)+"\n", remote)

		header := runGit(t, dir, "config", "http.extraHeader")
		assert.Equal(t, "Authorization: Bearer test-token\n", header)
	})

	t.Run("is idempotent and refreshes remote url on an existing repo", func(t *testing.T) {
		cfg, _ := newTestConfig(t, "host-a", "-home-orfeo42")

		require.NoError(t, ensureStagingRepo(context.Background(), cfg))
		cfg.Token = "rotated-token"
		require.NoError(t, ensureStagingRepo(context.Background(), cfg))

		dir := stagingDir(cfg)
		header := runGit(t, dir, "config", "http.extraHeader")
		assert.Equal(t, "Authorization: Bearer rotated-token\n", header)
	})
}

func TestSyncWorktree(t *testing.T) {
	t.Run("writes local files into the staging directory", func(t *testing.T) {
		dir := t.TempDir()
		srcDir := t.TempDir()
		srcPath := filepath.Join(srcDir, "CLAUDE.md")
		require.NoError(t, os.WriteFile(srcPath, []byte("hello"), 0o644))

		local := manifest.Manifest{{Path: "global/CLAUDE.md", SHA256: "h1", Size: 5}}
		localPaths := map[string]string{"global/CLAUDE.md": srcPath}

		require.NoError(t, syncWorktree(dir, local, localPaths))

		content, err := os.ReadFile(filepath.Join(dir, "global", "CLAUDE.md"))
		require.NoError(t, err)
		assert.Equal(t, "hello", string(content))
	})

	t.Run("removes staging files no longer present in local manifest", func(t *testing.T) {
		dir := t.TempDir()
		stalePath := filepath.Join(dir, "global", "stale.md")
		require.NoError(t, os.MkdirAll(filepath.Dir(stalePath), 0o750))
		require.NoError(t, os.WriteFile(stalePath, []byte("old"), 0o644))

		require.NoError(t, syncWorktree(dir, manifest.Manifest{}, map[string]string{}))

		_, statErr := os.Stat(stalePath)
		assert.True(t, os.IsNotExist(statErr))
	})

	t.Run("leaves the top-level .git directory untouched", func(t *testing.T) {
		dir := t.TempDir()
		gitDir := filepath.Join(dir, ".git")
		require.NoError(t, os.MkdirAll(gitDir, 0o750))
		keepPath := filepath.Join(gitDir, "HEAD")
		require.NoError(t, os.WriteFile(keepPath, []byte("ref: refs/heads/main"), 0o644))

		require.NoError(t, syncWorktree(dir, manifest.Manifest{}, map[string]string{}))

		assert.FileExists(t, keepPath)
	})
}

func TestCommitAndPush(t *testing.T) {
	t.Run("does nothing when there are no staged changes", func(t *testing.T) {
		cfg, base := newTestConfig(t, "host-a", "-home-orfeo42")
		require.NoError(t, ensureStagingRepo(context.Background(), cfg))

		require.NoError(t, commitAndPush(context.Background(), stagingDir(cfg)))

		clientDir := clientBareDir(base, "host-a")
		out := runGit(t, "", "--git-dir", clientDir, "rev-list", "--all")
		assert.Empty(t, out)
	})

	t.Run("commits and pushes staged changes", func(t *testing.T) {
		cfg, base := newTestConfig(t, "host-b", "-home-orfeo42")
		require.NoError(t, ensureStagingRepo(context.Background(), cfg))

		dir := stagingDir(cfg)
		require.NoError(t, os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte("hello"), 0o644))

		require.NoError(t, commitAndPush(context.Background(), dir))

		clientDir := clientBareDir(base, "host-b")
		assert.Equal(t, 1, bareCommitCount(t, clientDir))
		assert.Contains(t, bareLsTree(t, clientDir), "CLAUDE.md")
	})

	t.Run("retries a previously rejected push when nothing new is staged", func(t *testing.T) {
		cfg, base := newTestConfig(t, "host-c", "-home-orfeo42")
		require.NoError(t, ensureStagingRepo(context.Background(), cfg))

		clientDir := clientBareDir(base, "host-c")
		hookPath := filepath.Join(clientDir, "hooks", "pre-receive")
		require.NoError(t, os.WriteFile(hookPath, []byte("#!/bin/sh\nexit 1\n"), 0o755))

		dir := stagingDir(cfg)
		require.NoError(t, os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte("hello"), 0o644))
		require.Error(t, commitAndPush(context.Background(), dir))
		assert.Empty(t, runGit(t, "", "--git-dir", clientDir, "rev-list", "--all"))

		require.NoError(t, os.Remove(hookPath))

		require.NoError(t, commitAndPush(context.Background(), dir))
		assert.Equal(t, 1, bareCommitCount(t, clientDir))
		assert.Contains(t, bareLsTree(t, clientDir), "CLAUDE.md")
	})
}
