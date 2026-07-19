package syncer

import (
	"path/filepath"
	"testing"

	"github.com/Orfeo42/claude-memory-sync/internal/manifest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadManifest(t *testing.T) {
	t.Run("returns empty manifest when state file does not exist", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "missing.json")

		got, err := loadManifest(path)
		require.NoError(t, err)
		assert.Empty(t, got)
	})

	t.Run("round-trips a saved manifest", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "state", "client.json")
		want := manifest.Manifest{{Path: "global/CLAUDE.md", SHA256: "abc", Size: 5}}

		require.NoError(t, saveManifest(path, want))

		got, err := loadManifest(path)
		require.NoError(t, err)
		assert.Equal(t, want, got)
	})
}

func TestSaveManifest(t *testing.T) {
	t.Run("creates parent directories", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "nested", "dir", "client.json")

		err := saveManifest(path, manifest.Manifest{})
		require.NoError(t, err)
		assert.FileExists(t, path)
	})
}
