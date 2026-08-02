package guardrails_test

import (
	"testing"

	"claude-memory-sync/internal/guardrails"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeFilename(t *testing.T) {
	t.Run("lowercases uppercase input", func(t *testing.T) {
		got, err := guardrails.NormalizeFilename("MyEntry.md")
		require.NoError(t, err)
		assert.Equal(t, "myentry.md", got)
	})

	t.Run("converts underscores to dashes", func(t *testing.T) {
		got, err := guardrails.NormalizeFilename("my_entry_name.md")
		require.NoError(t, err)
		assert.Equal(t, "my-entry-name.md", got)
	})

	t.Run("accepts an already normalized name", func(t *testing.T) {
		got, err := guardrails.NormalizeFilename("already-normalized.md")
		require.NoError(t, err)
		assert.Equal(t, "already-normalized.md", got)
	})

	t.Run("rejects a name containing spaces", func(t *testing.T) {
		_, err := guardrails.NormalizeFilename("my entry.md")
		require.Error(t, err)
		assert.ErrorIs(t, err, guardrails.ErrInvalidFilename)
	})

	t.Run("rejects a name missing the .md extension", func(t *testing.T) {
		_, err := guardrails.NormalizeFilename("my-entry")
		require.Error(t, err)
		assert.ErrorIs(t, err, guardrails.ErrInvalidFilename)
	})

	t.Run("rejects a name with double dashes", func(t *testing.T) {
		_, err := guardrails.NormalizeFilename("my--entry.md")
		require.Error(t, err)
		assert.ErrorIs(t, err, guardrails.ErrInvalidFilename)
	})

	t.Run("rejects memory.md", func(t *testing.T) {
		_, err := guardrails.NormalizeFilename("memory.md")
		require.Error(t, err)
		assert.ErrorIs(t, err, guardrails.ErrIndexTarget)
	})

	t.Run("rejects a normalized form of memory.md", func(t *testing.T) {
		_, err := guardrails.NormalizeFilename("MEMORY.md")
		require.Error(t, err)
		assert.ErrorIs(t, err, guardrails.ErrIndexTarget)
	})
}
