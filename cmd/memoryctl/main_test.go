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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newEnsureWorkFixture(t *testing.T) (string, string, string, string) {
	t.Helper()

	reposDir := filepath.Join(t.TempDir(), "repos")
	bare := filepath.Join(reposDir, "canonical.git")
	initBareRepo(t, bare)
	commitFiles(t, bare, map[string]string{"README.md": "seed"}, "seed", time.Now())

	workDir := filepath.Join(t.TempDir(), "work")
	lock := filepath.Join(t.TempDir(), "canonical.lock")

	return reposDir, bare, workDir, lock
}

func ensureWorkArgs(reposDir, workDir, lock string) []string {
	return []string{"--repos-dir", reposDir, "--work-dir", workDir, "--lock", lock}
}

func headOf(t *testing.T, dir string) string {
	t.Helper()
	return strings.TrimSpace(runGit(t, dir, nil, "rev-parse", "HEAD"))
}

func localCommit(t *testing.T, workDir, name string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(workDir, name), []byte(name), 0o600))
	runGit(t, workDir, nil, "add", "-A")
	runGit(t, workDir, nil, "commit", "-m", name)
}

func TestCmdEnsureWork(t *testing.T) {
	ctx := context.Background()

	t.Run("fresh clones the canonical repo", func(t *testing.T) {
		reposDir, bare, workDir, lock := newEnsureWorkFixture(t)

		require.NoError(t, cmdEnsureWork(ctx, ensureWorkArgs(reposDir, workDir, lock)))

		assert.FileExists(t, filepath.Join(workDir, "README.md"))
		assert.Equal(t, strings.TrimSpace(runGit(t, bare, nil, "rev-parse", "main")), headOf(t, workDir))
		assert.Equal(t, "memory-synthesizer", strings.TrimSpace(runGit(t, workDir, nil, "config", "user.name")))
	})

	t.Run("behind is fast-forwarded to origin main", func(t *testing.T) {
		reposDir, bare, workDir, lock := newEnsureWorkFixture(t)
		require.NoError(t, cmdEnsureWork(ctx, ensureWorkArgs(reposDir, workDir, lock)))

		newHead := commitFiles(t, bare, map[string]string{"new.md": "new"}, "new", time.Now())

		require.NoError(t, cmdEnsureWork(ctx, ensureWorkArgs(reposDir, workDir, lock)))

		assert.Equal(t, newHead, headOf(t, workDir))
		assert.FileExists(t, filepath.Join(workDir, "new.md"))
	})

	t.Run("equal and dirty is cleaned", func(t *testing.T) {
		reposDir, _, workDir, lock := newEnsureWorkFixture(t)
		require.NoError(t, cmdEnsureWork(ctx, ensureWorkArgs(reposDir, workDir, lock)))
		require.NoError(t, os.WriteFile(filepath.Join(workDir, "README.md"), []byte("dirty"), 0o600))

		require.NoError(t, cmdEnsureWork(ctx, ensureWorkArgs(reposDir, workDir, lock)))

		content, err := os.ReadFile(filepath.Join(workDir, "README.md"))
		require.NoError(t, err)
		assert.Equal(t, "seed", string(content))
	})

	t.Run("ahead keeps unpushed commit and discards junk", func(t *testing.T) {
		reposDir, _, workDir, lock := newEnsureWorkFixture(t)
		require.NoError(t, cmdEnsureWork(ctx, ensureWorkArgs(reposDir, workDir, lock)))
		localCommit(t, workDir, "unpushed.md")
		head := headOf(t, workDir)
		require.NoError(t, os.WriteFile(filepath.Join(workDir, "junk.md"), []byte("junk"), 0o600))
		require.NoError(t, os.WriteFile(filepath.Join(workDir, "README.md"), []byte("dirty"), 0o600))

		require.NoError(t, cmdEnsureWork(ctx, ensureWorkArgs(reposDir, workDir, lock)))

		assert.Equal(t, head, headOf(t, workDir))
		assert.FileExists(t, filepath.Join(workDir, "unpushed.md"))
		assert.NoFileExists(t, filepath.Join(workDir, "junk.md"))
		content, err := os.ReadFile(filepath.Join(workDir, "README.md"))
		require.NoError(t, err)
		assert.Equal(t, "seed", string(content))
	})

	t.Run("diverged returns conflict and leaves HEAD untouched", func(t *testing.T) {
		reposDir, bare, workDir, lock := newEnsureWorkFixture(t)
		require.NoError(t, cmdEnsureWork(ctx, ensureWorkArgs(reposDir, workDir, lock)))
		localCommit(t, workDir, "local.md")
		head := headOf(t, workDir)
		commitFiles(t, bare, map[string]string{"remote.md": "remote"}, "remote", time.Now())

		err := cmdEnsureWork(ctx, ensureWorkArgs(reposDir, workDir, lock))

		require.ErrorIs(t, err, domain.ErrConflict)
		assert.Equal(t, head, headOf(t, workDir))
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
		case <-done:
			t.Fatal("ensure-work finished while lock was held")
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
