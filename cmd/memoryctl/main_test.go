package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"claude-memory-sync/internal/domain"
	"claude-memory-sync/internal/gitutil/gittest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newEnsureWorkFixture(t *testing.T) (string, string, string, string) {
	t.Helper()

	reposDir := filepath.Join(t.TempDir(), "repos")
	bare := filepath.Join(reposDir, "canonical.git")
	gittest.InitBareRepo(t, bare)
	gittest.CommitFiles(t, bare, map[string]string{"README.md": "seed"}, "seed", time.Now())

	workDir := filepath.Join(t.TempDir(), "work")
	lock := filepath.Join(t.TempDir(), "canonical.lock")

	return reposDir, bare, workDir, lock
}

func ensureWorkArgs(reposDir, workDir, lock string) []string {
	return []string{"--repos-dir", reposDir, "--work-dir", workDir, "--lock", lock}
}

func headOf(t *testing.T, dir string) string {
	t.Helper()
	return strings.TrimSpace(gittest.RunGit(t, dir, nil, "rev-parse", "HEAD"))
}

func TestCmdEnsureWork(t *testing.T) {
	ctx := context.Background()

	t.Run("fresh clones the canonical repo", func(t *testing.T) {
		reposDir, bare, workDir, lock := newEnsureWorkFixture(t)

		require.NoError(t, cmdEnsureWork(ctx, ensureWorkArgs(reposDir, workDir, lock)))

		assert.FileExists(t, filepath.Join(workDir, "README.md"))
		assert.Equal(t, strings.TrimSpace(gittest.RunGit(t, bare, nil, "rev-parse", "main")), headOf(t, workDir))
		assert.Equal(t, "memory-synthesizer", strings.TrimSpace(gittest.RunGit(t, workDir, nil, "config", "user.name")))
	})

	t.Run("missing lock flag returns validation error", func(t *testing.T) {
		reposDir, _, workDir, _ := newEnsureWorkFixture(t)

		err := cmdEnsureWork(ctx, []string{"--repos-dir", reposDir, "--work-dir", workDir})

		require.ErrorIs(t, err, domain.ErrValidation)
		assert.NoDirExists(t, workDir)
	})

	t.Run("waits for the lock holder", func(t *testing.T) {
		reposDir, _, workDir, lock := newEnsureWorkFixture(t)
		file, err := os.OpenFile(lock, os.O_CREATE|os.O_RDWR, 0o600)
		require.NoError(t, err)
		defer func() { require.NoError(t, file.Close()) }()
		require.NoError(t, syscall.Flock(int(file.Fd()), syscall.LOCK_EX))

		done := make(chan error, 1)
		go func() {
			done <- cmdEnsureWork(ctx, ensureWorkArgs(reposDir, workDir, lock))
		}()

		select {
		case doneErr := <-done:
			t.Fatalf("ensure-work finished while lock was held: %v", doneErr)
		case <-time.After(300 * time.Millisecond):
		}

		require.NoError(t, syscall.Flock(int(file.Fd()), syscall.LOCK_UN))

		select {
		case err := <-done:
			require.NoError(t, err)
		case <-time.After(10 * time.Second):
			t.Fatal("ensure-work did not finish after lock release")
		}
	})
}
