package workrepo

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"claude-memory-sync/internal/domain"
	"claude-memory-sync/internal/gitutil/gittest"
)

func newEnsureWorkFixture(t *testing.T) (string, string, string) {
	t.Helper()

	reposDir := filepath.Join(t.TempDir(), "repos")
	bare := filepath.Join(reposDir, "canonical.git")
	gittest.InitBareRepo(t, bare)
	gittest.CommitFiles(t, bare, map[string]string{"README.md": "seed"}, "seed", time.Now())

	workDir := filepath.Join(t.TempDir(), "work")

	return reposDir, bare, workDir
}

func headOf(t *testing.T, dir string) string {
	t.Helper()
	return strings.TrimSpace(gittest.RunGit(t, dir, nil, "rev-parse", "HEAD"))
}

func localCommit(t *testing.T, workDir, name string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(workDir, name), []byte(name), 0o600))
	gittest.RunGit(t, workDir, nil, "add", "-A")
	gittest.RunGit(t, workDir, nil, "commit", "-m", name)
}

func TestEnsureWork(t *testing.T) {
	ctx := context.Background()

	t.Run("fresh clones the canonical repo", func(t *testing.T) {
		reposDir, bare, workDir := newEnsureWorkFixture(t)

		require.NoError(t, EnsureWork(ctx, reposDir, workDir))

		assert.FileExists(t, filepath.Join(workDir, "README.md"))
		assert.Equal(t, strings.TrimSpace(gittest.RunGit(t, bare, nil, "rev-parse", "main")), headOf(t, workDir))
		assert.Equal(t, "memory-synthesizer", strings.TrimSpace(gittest.RunGit(t, workDir, nil, "config", "user.name")))
	})

	t.Run("behind is fast-forwarded to origin main", func(t *testing.T) {
		reposDir, bare, workDir := newEnsureWorkFixture(t)
		require.NoError(t, EnsureWork(ctx, reposDir, workDir))

		newHead := gittest.CommitFiles(t, bare, map[string]string{"new.md": "new"}, "new", time.Now())

		require.NoError(t, EnsureWork(ctx, reposDir, workDir))

		assert.Equal(t, newHead, headOf(t, workDir))
		assert.FileExists(t, filepath.Join(workDir, "new.md"))
	})

	t.Run("equal and dirty is cleaned", func(t *testing.T) {
		reposDir, _, workDir := newEnsureWorkFixture(t)
		require.NoError(t, EnsureWork(ctx, reposDir, workDir))
		require.NoError(t, os.WriteFile(filepath.Join(workDir, "README.md"), []byte("dirty"), 0o600))

		require.NoError(t, EnsureWork(ctx, reposDir, workDir))

		content, err := os.ReadFile(filepath.Join(workDir, "README.md"))
		require.NoError(t, err)
		assert.Equal(t, "seed", string(content))
	})

	t.Run("ahead keeps unpushed commit and discards junk", func(t *testing.T) {
		reposDir, _, workDir := newEnsureWorkFixture(t)
		require.NoError(t, EnsureWork(ctx, reposDir, workDir))
		localCommit(t, workDir, "unpushed.md")
		head := headOf(t, workDir)
		require.NoError(t, os.WriteFile(filepath.Join(workDir, "junk.md"), []byte("junk"), 0o600))
		require.NoError(t, os.WriteFile(filepath.Join(workDir, "README.md"), []byte("dirty"), 0o600))

		require.NoError(t, EnsureWork(ctx, reposDir, workDir))

		assert.Equal(t, head, headOf(t, workDir))
		assert.FileExists(t, filepath.Join(workDir, "unpushed.md"))
		assert.NoFileExists(t, filepath.Join(workDir, "junk.md"))
		content, err := os.ReadFile(filepath.Join(workDir, "README.md"))
		require.NoError(t, err)
		assert.Equal(t, "seed", string(content))
	})

	t.Run("diverged returns conflict and leaves HEAD untouched", func(t *testing.T) {
		reposDir, bare, workDir := newEnsureWorkFixture(t)
		require.NoError(t, EnsureWork(ctx, reposDir, workDir))
		localCommit(t, workDir, "local.md")
		head := headOf(t, workDir)
		gittest.CommitFiles(t, bare, map[string]string{"remote.md": "remote"}, "remote", time.Now())

		err := EnsureWork(ctx, reposDir, workDir)

		require.ErrorIs(t, err, domain.ErrConflict)
		assert.Equal(t, head, headOf(t, workDir))
	})
}
