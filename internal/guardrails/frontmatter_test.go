package guardrails_test

import (
	"testing"

	"claude-memory-sync/internal/guardrails"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateFrontmatter(t *testing.T) {
	t.Run("accepts a fully valid frontmatter block", func(t *testing.T) {
		content := []byte("---\nname: My Entry\ndescription: Something useful\nmetadata:\n  type: fact\n---\nbody\n")
		err := guardrails.ValidateFrontmatter(content)
		require.NoError(t, err)
	})

	t.Run("rejects content with no frontmatter at all", func(t *testing.T) {
		content := []byte("just a body\nno frontmatter here\n")
		err := guardrails.ValidateFrontmatter(content)
		require.Error(t, err)
		assert.ErrorIs(t, err, guardrails.ErrNoFrontmatter)
	})

	t.Run("rejects an unclosed frontmatter block", func(t *testing.T) {
		content := []byte("---\nname: My Entry\ndescription: Something\n")
		err := guardrails.ValidateFrontmatter(content)
		require.Error(t, err)
		assert.ErrorIs(t, err, guardrails.ErrNoFrontmatter)
	})

	t.Run("rejects a missing name field", func(t *testing.T) {
		content := []byte("---\ndescription: Something useful\nmetadata:\n  type: fact\n---\n")
		err := guardrails.ValidateFrontmatter(content)
		require.Error(t, err)
		assert.ErrorIs(t, err, guardrails.ErrMissingName)
	})

	t.Run("rejects an empty name field", func(t *testing.T) {
		content := []byte("---\nname:\ndescription: Something useful\nmetadata:\n  type: fact\n---\n")
		err := guardrails.ValidateFrontmatter(content)
		require.Error(t, err)
		assert.ErrorIs(t, err, guardrails.ErrMissingName)
	})

	t.Run("rejects a missing description field", func(t *testing.T) {
		content := []byte("---\nname: My Entry\nmetadata:\n  type: fact\n---\n")
		err := guardrails.ValidateFrontmatter(content)
		require.Error(t, err)
		assert.ErrorIs(t, err, guardrails.ErrMissingDescription)
	})

	t.Run("rejects a missing metadata type", func(t *testing.T) {
		content := []byte("---\nname: My Entry\ndescription: Something useful\n---\n")
		err := guardrails.ValidateFrontmatter(content)
		require.Error(t, err)
		assert.ErrorIs(t, err, guardrails.ErrMissingMetadataType)
	})

	t.Run("rejects an empty metadata type", func(t *testing.T) {
		content := []byte("---\nname: My Entry\ndescription: Something useful\nmetadata:\n  type:\n---\n")
		err := guardrails.ValidateFrontmatter(content)
		require.Error(t, err)
		assert.ErrorIs(t, err, guardrails.ErrMissingMetadataType)
	})

	t.Run("accepts double-quoted values and strips the quotes", func(t *testing.T) {
		content := []byte("---\nname: \"My Entry\"\ndescription: \"Something useful\"\nmetadata:\n  type: \"fact\"\n---\n")
		err := guardrails.ValidateFrontmatter(content)
		require.NoError(t, err)
	})

	t.Run("accepts single-quoted values and strips the quotes", func(t *testing.T) {
		content := []byte("---\nname: 'My Entry'\ndescription: 'Something useful'\nmetadata:\n  type: 'fact'\n---\n")
		err := guardrails.ValidateFrontmatter(content)
		require.NoError(t, err)
	})
}
