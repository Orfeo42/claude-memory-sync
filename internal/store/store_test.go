package store_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"claude-memory-sync/internal/store"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func gitLog(t *testing.T, root string) string {
	t.Helper()
	cmd := exec.Command("git", "-C", root, "log", "--oneline")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err)
	return string(out)
}

func TestNew(t *testing.T) {
	t.Run("initializes git repo with canonical and clients dirs", func(t *testing.T) {
		root := t.TempDir()

		_, err := store.New(t.Context(), root)
		require.NoError(t, err)

		assert.DirExists(t, filepath.Join(root, ".git"))
		assert.DirExists(t, filepath.Join(root, "canonical"))
		assert.DirExists(t, filepath.Join(root, "clients"))
		assert.FileExists(t, filepath.Join(root, "canonical", ".gitkeep"))
		assert.FileExists(t, filepath.Join(root, "clients", ".gitkeep"))

		log := gitLog(t, root)
		assert.Contains(t, log, "init: memory-server storage")
	})

	t.Run("idempotent when called on an already initialized store", func(t *testing.T) {
		root := t.TempDir()

		_, err := store.New(t.Context(), root)
		require.NoError(t, err)

		_, err = store.New(t.Context(), root)
		require.NoError(t, err)

		log := gitLog(t, root)
		lines := strings.Split(strings.TrimSpace(log), "\n")
		assert.Len(t, lines, 1)
	})
}

func TestGitStoreTree(t *testing.T) {
	t.Run("returns manifest of files in namespace", func(t *testing.T) {
		root := t.TempDir()
		s, err := store.New(t.Context(), root)
		require.NoError(t, err)

		require.NoError(t, s.Write(t.Context(), "clients/host-a", "global/CLAUDE.md", []byte("hello"), "host-a"))

		tree, err := s.Tree(t.Context(), "clients/host-a")
		require.NoError(t, err)
		require.Len(t, tree, 1)
		assert.Equal(t, "global/CLAUDE.md", tree[0].Path)
	})

	t.Run("returns empty manifest for namespace with no files yet", func(t *testing.T) {
		root := t.TempDir()
		s, err := store.New(t.Context(), root)
		require.NoError(t, err)

		tree, err := s.Tree(t.Context(), "clients/host-b")
		require.NoError(t, err)
		assert.Empty(t, tree)
	})

	t.Run("rejects invalid namespace", func(t *testing.T) {
		root := t.TempDir()
		s, err := store.New(t.Context(), root)
		require.NoError(t, err)

		_, err = s.Tree(t.Context(), "../etc")
		require.ErrorIs(t, err, store.ErrInvalidNamespace)
	})
}

func TestGitStoreWrite(t *testing.T) {
	t.Run("writes file content and commits", func(t *testing.T) {
		root := t.TempDir()
		s, err := store.New(t.Context(), root)
		require.NoError(t, err)

		err = s.Write(t.Context(), "clients/host-a", "global/CLAUDE.md", []byte("hello"), "host-a")
		require.NoError(t, err)

		content, err := os.ReadFile(filepath.Join(root, "clients", "host-a", "global", "CLAUDE.md"))
		require.NoError(t, err)
		assert.Equal(t, "hello", string(content))

		log := gitLog(t, root)
		assert.Contains(t, log, "sync: host-a clients/host-a/global/CLAUDE.md")
	})

	t.Run("second write with identical content is a no-op commit", func(t *testing.T) {
		root := t.TempDir()
		s, err := store.New(t.Context(), root)
		require.NoError(t, err)

		require.NoError(t, s.Write(t.Context(), "clients/host-a", "global/CLAUDE.md", []byte("hello"), "host-a"))
		logBefore := gitLog(t, root)

		require.NoError(t, s.Write(t.Context(), "clients/host-a", "global/CLAUDE.md", []byte("hello"), "host-a"))
		logAfter := gitLog(t, root)

		assert.Equal(t, logBefore, logAfter)
	})

	t.Run("rejects path traversal", func(t *testing.T) {
		root := t.TempDir()
		s, err := store.New(t.Context(), root)
		require.NoError(t, err)

		err = s.Write(t.Context(), "clients/host-a", "../../etc/passwd", []byte("x"), "host-a")
		require.ErrorIs(t, err, store.ErrInvalidPath)
	})

	t.Run("rejects absolute path", func(t *testing.T) {
		root := t.TempDir()
		s, err := store.New(t.Context(), root)
		require.NoError(t, err)

		err = s.Write(t.Context(), "clients/host-a", "/etc/passwd", []byte("x"), "host-a")
		require.ErrorIs(t, err, store.ErrInvalidPath)
	})
}

func TestGitStoreRead(t *testing.T) {
	t.Run("reads existing file content", func(t *testing.T) {
		root := t.TempDir()
		s, err := store.New(t.Context(), root)
		require.NoError(t, err)
		require.NoError(t, s.Write(t.Context(), "clients/host-a", "global/CLAUDE.md", []byte("hello"), "host-a"))

		content, err := s.Read(t.Context(), "clients/host-a", "global/CLAUDE.md")
		require.NoError(t, err)
		assert.Equal(t, "hello", string(content))
	})

	t.Run("returns ErrNotFound for missing file", func(t *testing.T) {
		root := t.TempDir()
		s, err := store.New(t.Context(), root)
		require.NoError(t, err)

		_, err = s.Read(t.Context(), "clients/host-a", "global/CLAUDE.md")
		require.ErrorIs(t, err, store.ErrNotFound)
	})
}

func TestGitStoreDelete(t *testing.T) {
	t.Run("deletes existing file and commits", func(t *testing.T) {
		root := t.TempDir()
		s, err := store.New(t.Context(), root)
		require.NoError(t, err)
		require.NoError(t, s.Write(t.Context(), "clients/host-a", "global/CLAUDE.md", []byte("hello"), "host-a"))

		err = s.Delete(t.Context(), "clients/host-a", "global/CLAUDE.md", "host-a")
		require.NoError(t, err)

		_, statErr := os.Stat(filepath.Join(root, "clients", "host-a", "global", "CLAUDE.md"))
		assert.True(t, os.IsNotExist(statErr))

		log := gitLog(t, root)
		assert.Contains(t, log, "sync: host-a clients/host-a/global/CLAUDE.md")
	})

	t.Run("deleting a missing file is a no-op", func(t *testing.T) {
		root := t.TempDir()
		s, err := store.New(t.Context(), root)
		require.NoError(t, err)

		logBefore := gitLog(t, root)

		err = s.Delete(t.Context(), "clients/host-a", "global/CLAUDE.md", "host-a")
		require.NoError(t, err)

		logAfter := gitLog(t, root)
		assert.Equal(t, logBefore, logAfter)
	})
}
