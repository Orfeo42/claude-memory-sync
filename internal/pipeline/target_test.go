package pipeline

import (
	"testing"

	"claude-memory-sync/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateTarget(t *testing.T) {
	t.Run("accepts global CLAUDE.md", func(t *testing.T) {
		require.NoError(t, ValidateTarget("global/CLAUDE.md"))
	})

	t.Run("accepts markdown rule", func(t *testing.T) {
		require.NoError(t, ValidateTarget("global/rules/go.md"))
	})

	t.Run("rejects non markdown rule", func(t *testing.T) {
		require.ErrorIs(t, ValidateTarget("global/rules/go.txt"), domain.ErrValidation)
	})

	t.Run("accepts skill file", func(t *testing.T) {
		require.NoError(t, ValidateTarget("global/skills/foo/run.sh"))
	})

	t.Run("accepts agent file", func(t *testing.T) {
		require.NoError(t, ValidateTarget("global/agents/impl.md"))
	})

	t.Run("accepts project memory entry", func(t *testing.T) {
		require.NoError(t, ValidateTarget("projects/p/memory/note.md"))
	})

	t.Run("rejects project MEMORY.md index", func(t *testing.T) {
		require.ErrorIs(t, ValidateTarget("projects/p/memory/MEMORY.md"), domain.ErrValidation)
	})

	t.Run("rejects project non markdown entry", func(t *testing.T) {
		require.ErrorIs(t, ValidateTarget("projects/p/memory/note.txt"), domain.ErrValidation)
	})

	t.Run("rejects nested project memory path", func(t *testing.T) {
		require.ErrorIs(t, ValidateTarget("projects/p/memory/sub/note.md"), domain.ErrValidation)
	})

	t.Run("rejects empty path", func(t *testing.T) {
		require.ErrorIs(t, ValidateTarget(""), domain.ErrValidation)
	})

	t.Run("rejects traversal", func(t *testing.T) {
		require.ErrorIs(t, ValidateTarget("global/rules/../../x.md"), domain.ErrValidation)
	})

	t.Run("rejects absolute path", func(t *testing.T) {
		require.ErrorIs(t, ValidateTarget("/global/rules/x.md"), domain.ErrValidation)
	})

	t.Run("rejects unclean path", func(t *testing.T) {
		require.ErrorIs(t, ValidateTarget("global//rules/x.md"), domain.ErrValidation)
	})

	t.Run("rejects path outside whitelist", func(t *testing.T) {
		err := ValidateTarget("other/x.md")
		require.ErrorIs(t, err, domain.ErrValidation)
		assert.Contains(t, err.Error(), "not in an allowed location")
	})
}
