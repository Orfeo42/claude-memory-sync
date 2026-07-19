package manifest_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"claude-memory-sync/internal/manifest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuild(t *testing.T) {
	t.Run("hashes included files and skips filtered ones", func(t *testing.T) {
		root := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(root, "keep.md"), []byte("hello"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(root, "skip.txt"), []byte("nope"), 0o644))

		whitelist := func(relPath string, d fs.DirEntry) bool {
			if d.IsDir() {
				return true
			}
			return filepath.Ext(relPath) == ".md"
		}

		got, err := manifest.Build(root, whitelist)
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Equal(t, "keep.md", got[0].Path)
		assert.Equal(t, int64(len("hello")), got[0].Size)

		wantHash, err := manifest.HashFile(filepath.Join(root, "keep.md"))
		require.NoError(t, err)
		assert.Equal(t, wantHash, got[0].SHA256)
	})

	t.Run("returns empty manifest for missing root", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "does-not-exist")

		got, err := manifest.Build(root, func(string, fs.DirEntry) bool { return true })
		require.NoError(t, err)
		assert.Empty(t, got)
	})

	t.Run("skips symlinked files", func(t *testing.T) {
		root := t.TempDir()
		target := filepath.Join(root, "real.md")
		require.NoError(t, os.WriteFile(target, []byte("hello"), 0o644))
		link := filepath.Join(root, "link.md")
		require.NoError(t, os.Symlink(target, link))

		got, err := manifest.Build(root, func(string, fs.DirEntry) bool { return true })
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Equal(t, "real.md", got[0].Path)
	})

	t.Run("prunes directories rejected by whitelist", func(t *testing.T) {
		root := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(root, "skip-dir"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(root, "skip-dir", "file.md"), []byte("x"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(root, "top.md"), []byte("y"), 0o644))

		whitelist := func(relPath string, d fs.DirEntry) bool {
			if d.IsDir() {
				return relPath == "."
			}
			return true
		}

		got, err := manifest.Build(root, whitelist)
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Equal(t, "top.md", got[0].Path)
	})
}

func TestHashFile(t *testing.T) {
	t.Run("returns sha256 hex digest", func(t *testing.T) {
		root := t.TempDir()
		path := filepath.Join(root, "file.txt")
		require.NoError(t, os.WriteFile(path, []byte("hello"), 0o644))

		got, err := manifest.HashFile(path)
		require.NoError(t, err)
		assert.Equal(t, "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824", got)
	})

	t.Run("errors on missing file", func(t *testing.T) {
		_, err := manifest.HashFile(filepath.Join(t.TempDir(), "missing.txt"))
		require.Error(t, err)
	})
}
