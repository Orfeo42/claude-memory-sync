package store_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
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

func commitCount(t *testing.T, root string) int {
	t.Helper()
	cmd := exec.Command("git", "-C", root, "rev-list", "--count", "HEAD")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err)
	count, err := strconv.Atoi(strings.TrimSpace(string(out)))
	require.NoError(t, err)
	return count
}

func gitStatusPorcelain(t *testing.T, root string) string {
	t.Helper()
	cmd := exec.Command("git", "-C", root, "status", "--porcelain")
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

	t.Run("mirrors client write to canonical namespace", func(t *testing.T) {
		root := t.TempDir()
		s, err := store.New(t.Context(), root)
		require.NoError(t, err)

		err = s.Write(t.Context(), "clients/host-a", "global/CLAUDE.md", []byte("hello"), "host-a")
		require.NoError(t, err)

		clientContent, err := os.ReadFile(filepath.Join(root, "clients", "host-a", "global", "CLAUDE.md"))
		require.NoError(t, err)
		canonicalContent, err := os.ReadFile(filepath.Join(root, "canonical", "global", "CLAUDE.md"))
		require.NoError(t, err)
		assert.Equal(t, "hello", string(clientContent))
		assert.Equal(t, string(clientContent), string(canonicalContent))
	})

	t.Run("writing to canonical namespace directly does not create clients files", func(t *testing.T) {
		root := t.TempDir()
		s, err := store.New(t.Context(), root)
		require.NoError(t, err)

		err = s.Write(t.Context(), "canonical", "global/CLAUDE.md", []byte("hello"), "host-a")
		require.NoError(t, err)

		assert.FileExists(t, filepath.Join(root, "canonical", "global", "CLAUDE.md"))
		_, statErr := os.Stat(filepath.Join(root, "clients"))
		require.NoError(t, statErr)
		entries, err := os.ReadDir(filepath.Join(root, "clients"))
		require.NoError(t, err)
		for _, entry := range entries {
			assert.Equal(t, ".gitkeep", entry.Name())
		}
	})

	t.Run("client write with mirror is a single atomic commit", func(t *testing.T) {
		root := t.TempDir()
		s, err := store.New(t.Context(), root)
		require.NoError(t, err)

		countBefore := commitCount(t, root)

		err = s.Write(t.Context(), "clients/host-a", "global/CLAUDE.md", []byte("hello"), "host-a")
		require.NoError(t, err)

		countAfter := commitCount(t, root)
		assert.Equal(t, countBefore+1, countAfter)
		assert.Empty(t, gitStatusPorcelain(t, root))
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

	t.Run("deleting client file also removes canonical copy", func(t *testing.T) {
		root := t.TempDir()
		s, err := store.New(t.Context(), root)
		require.NoError(t, err)
		require.NoError(t, s.Write(t.Context(), "clients/host-a", "global/CLAUDE.md", []byte("hello"), "host-a"))

		err = s.Delete(t.Context(), "clients/host-a", "global/CLAUDE.md", "host-a")
		require.NoError(t, err)

		_, clientStatErr := os.Stat(filepath.Join(root, "clients", "host-a", "global", "CLAUDE.md"))
		assert.True(t, os.IsNotExist(clientStatErr))
		_, canonicalStatErr := os.Stat(filepath.Join(root, "canonical", "global", "CLAUDE.md"))
		assert.True(t, os.IsNotExist(canonicalStatErr))
		assert.Empty(t, gitStatusPorcelain(t, root))
	})

	t.Run("deleting client file when canonical copy already absent succeeds", func(t *testing.T) {
		root := t.TempDir()
		s, err := store.New(t.Context(), root)
		require.NoError(t, err)
		require.NoError(t, s.Write(t.Context(), "clients/host-a", "global/CLAUDE.md", []byte("hello"), "host-a"))
		require.NoError(t, s.Delete(t.Context(), "canonical", "global/CLAUDE.md", "host-a"))

		err = s.Delete(t.Context(), "clients/host-a", "global/CLAUDE.md", "host-a")
		require.NoError(t, err)

		_, clientStatErr := os.Stat(filepath.Join(root, "clients", "host-a", "global", "CLAUDE.md"))
		assert.True(t, os.IsNotExist(clientStatErr))
		assert.Empty(t, gitStatusPorcelain(t, root))
	})

	t.Run("deleting when client copy absent but canonical present removes canonical", func(t *testing.T) {
		root := t.TempDir()
		s, err := store.New(t.Context(), root)
		require.NoError(t, err)
		require.NoError(t, s.Write(t.Context(), "canonical", "global/CLAUDE.md", []byte("hello"), "host-a"))

		err = s.Delete(t.Context(), "clients/host-a", "global/CLAUDE.md", "host-a")
		require.NoError(t, err)

		_, canonicalStatErr := os.Stat(filepath.Join(root, "canonical", "global", "CLAUDE.md"))
		assert.True(t, os.IsNotExist(canonicalStatErr))
		assert.Empty(t, gitStatusPorcelain(t, root))
	})
}
