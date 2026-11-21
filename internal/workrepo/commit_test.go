package workrepo

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"claude-memory-sync/internal/gitutil/gittest"
)

func newTestWorkDir(t *testing.T) string {
	t.Helper()

	repoDir := filepath.Join(t.TempDir(), "canonical.git")
	gittest.InitBareRepo(t, repoDir)

	workDir := t.TempDir()
	gittest.RunGit(t, "", nil, "clone", repoDir, workDir)
	gittest.RunGit(t, workDir, nil, "config", "user.name", "memory-synthesizer")
	gittest.RunGit(t, workDir, nil, "config", "user.email", "memory-synthesizer@local")

	require.NoError(t, os.WriteFile(filepath.Join(workDir, "README.md"), []byte("seed"), 0o600))
	gittest.RunGit(t, workDir, nil, "add", "-A")
	gittest.RunGit(t, workDir, nil, "commit", "-m", "seed")
	gittest.RunGit(t, workDir, nil, "push", "origin", "main")

	return workDir
}

func TestCommit(t *testing.T) {
	t.Run("no changes returns false", func(t *testing.T) {
		workDir := newTestWorkDir(t)
		lock := filepath.Join(t.TempDir(), "canonical.lock")

		committed, err := Commit(context.Background(), CommitOptions{WorkDir: workDir, Lock: lock, Message: "no-op", Pathspec: "."})
		require.NoError(t, err)
		assert.False(t, committed)
	})

	t.Run("staged changes are committed", func(t *testing.T) {
		workDir := newTestWorkDir(t)
		lock := filepath.Join(t.TempDir(), "canonical.lock")

		require.NoError(t, os.WriteFile(filepath.Join(workDir, "projects.md"), []byte("new content"), 0o600))

		committed, err := Commit(context.Background(), CommitOptions{WorkDir: workDir, Lock: lock, Message: "add projects", Pathspec: "."})
		require.NoError(t, err)
		assert.True(t, committed)

		log := gittest.RunGit(t, workDir, nil, "log", "-1", "--format=%s")
		assert.Contains(t, log, "add projects")
	})

	t.Run("missing pathspec returns false", func(t *testing.T) {
		workDir := newTestWorkDir(t)
		lock := filepath.Join(t.TempDir(), "canonical.lock")

		committed, err := Commit(context.Background(), CommitOptions{WorkDir: workDir, Lock: lock, Message: "no-op", Pathspec: "projects/new/memory"})
		require.NoError(t, err)
		assert.False(t, committed)
	})

	t.Run("existing pathspec is committed", func(t *testing.T) {
		workDir := newTestWorkDir(t)
		lock := filepath.Join(t.TempDir(), "canonical.lock")

		memoryDir := filepath.Join(workDir, "projects", "new", "memory")
		require.NoError(t, os.MkdirAll(memoryDir, 0o750))
		require.NoError(t, os.WriteFile(filepath.Join(memoryDir, "foo.md"), []byte("x"), 0o600))

		committed, err := Commit(context.Background(), CommitOptions{WorkDir: workDir, Lock: lock, Message: "add foo", Pathspec: "projects/new/memory"})
		require.NoError(t, err)
		assert.True(t, committed)
	})
}

func TestPush(t *testing.T) {
	t.Run("pushes local commits to origin main", func(t *testing.T) {
		workDir := newTestWorkDir(t)
		lock := filepath.Join(t.TempDir(), "canonical.lock")
		require.NoError(t, os.WriteFile(filepath.Join(workDir, "new.md"), []byte("new"), 0o600))
		gittest.RunGit(t, workDir, nil, "add", "-A")
		gittest.RunGit(t, workDir, nil, "commit", "-m", "new")

		require.NoError(t, Push(context.Background(), workDir, lock))

		local := gittest.RunGit(t, workDir, nil, "rev-parse", "HEAD")
		remote := gittest.RunGit(t, workDir, nil, "rev-parse", "origin/main")
		assert.Equal(t, local, remote)
	})

	t.Run("failing push returns error", func(t *testing.T) {
		workDir := newTestWorkDir(t)
		lock := filepath.Join(t.TempDir(), "canonical.lock")
		gittest.RunGit(t, workDir, nil, "remote", "set-url", "origin", filepath.Join(t.TempDir(), "missing.git"))

		require.Error(t, Push(context.Background(), workDir, lock))
	})
}
