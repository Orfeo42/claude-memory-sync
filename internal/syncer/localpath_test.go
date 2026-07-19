package syncer

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLocalPath(t *testing.T) {
	t.Run("maps global CLAUDE.md", func(t *testing.T) {
		got, err := localPath("global/CLAUDE.md", "/claude", "-home-orfeo42")
		require.NoError(t, err)
		assert.Equal(t, filepath.Join("/claude", "CLAUDE.md"), got)
	})

	t.Run("maps global rules file", func(t *testing.T) {
		got, err := localPath("global/rules/go.md", "/claude", "-home-orfeo42")
		require.NoError(t, err)
		assert.Equal(t, filepath.Join("/claude", "rules", "go.md"), got)
	})

	t.Run("maps HOME project memory file back to slug prefix", func(t *testing.T) {
		got, err := localPath("projects/HOME/memory/notes.md", "/claude", "-home-orfeo42")
		require.NoError(t, err)
		assert.Equal(t, filepath.Join("/claude", "projects", "-home-orfeo42", "memory", "notes.md"), got)
	})

	t.Run("maps HOME-suffix project memory file back to prefixed slug", func(t *testing.T) {
		got, err := localPath("projects/HOME-my-project/memory/sub/a.md", "/claude", "-home-orfeo42")
		require.NoError(t, err)
		assert.Equal(t, filepath.Join("/claude", "projects", "-home-orfeo42-my-project", "memory", "sub", "a.md"), got)
	})

	t.Run("errors on unknown canonical key", func(t *testing.T) {
		_, err := localPath("projects/OTHER/memory/notes.md", "/claude", "-home-orfeo42")
		require.ErrorIs(t, err, errUnknownCanonicalKey)
	})

	t.Run("errors on unrecognized namespace path", func(t *testing.T) {
		_, err := localPath("unknown/path.md", "/claude", "-home-orfeo42")
		require.ErrorIs(t, err, errInvalidNamespacePath)
	})
}
