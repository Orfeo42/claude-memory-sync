package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestWorkDir(t *testing.T) string {
	t.Helper()

	repoDir := filepath.Join(t.TempDir(), "canonical.git")
	initBareRepo(t, repoDir)

	workDir := t.TempDir()
	runGit(t, "", nil, "clone", repoDir, workDir)
	runGit(t, workDir, nil, "config", "user.name", "memory-synthesizer")
	runGit(t, workDir, nil, "config", "user.email", "memory-synthesizer@local")

	require.NoError(t, os.WriteFile(filepath.Join(workDir, "README.md"), []byte("seed"), 0o600))
	runGit(t, workDir, nil, "add", "-A")
	runGit(t, workDir, nil, "commit", "-m", "seed")
	runGit(t, workDir, nil, "push", "origin", "main")

	return workDir
}

func TestRunCommit(t *testing.T) {
	t.Run("no changes returns false", func(t *testing.T) {
		workDir := newTestWorkDir(t)
		lock := filepath.Join(t.TempDir(), "canonical.lock")

		committed, err := runCommit(context.Background(), workDir, lock, "no-op", ".")
		require.NoError(t, err)
		assert.False(t, committed)
	})

	t.Run("staged changes are committed", func(t *testing.T) {
		workDir := newTestWorkDir(t)
		lock := filepath.Join(t.TempDir(), "canonical.lock")

		require.NoError(t, os.WriteFile(filepath.Join(workDir, "projects.md"), []byte("new content"), 0o600))

		committed, err := runCommit(context.Background(), workDir, lock, "add projects", ".")
		require.NoError(t, err)
		assert.True(t, committed)

		log := runGit(t, workDir, nil, "log", "-1", "--format=%s")
		assert.Contains(t, log, "add projects")
	})
}
