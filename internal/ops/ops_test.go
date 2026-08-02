package ops

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParse(t *testing.T) {
	t.Run("single FILE", func(t *testing.T) {
		input := "===FILE: notes.md===\nhello\nworld\n===END===\n"

		result, err := Parse(strings.NewReader(input))

		require.NoError(t, err)
		require.Len(t, result.Files, 1)
		assert.Equal(t, "notes.md", result.Files[0].Name)
		assert.Equal(t, "hello\nworld\n", string(result.Files[0].Content))
		assert.False(t, result.Nothing)
	})

	t.Run("multiple FILE", func(t *testing.T) {
		input := "===FILE: a.md===\ncontent a\n===END===\n===FILE: b.md===\ncontent b\n===END===\n"

		result, err := Parse(strings.NewReader(input))

		require.NoError(t, err)
		require.Len(t, result.Files, 2)
		assert.Equal(t, "a.md", result.Files[0].Name)
		assert.Equal(t, "content a\n", string(result.Files[0].Content))
		assert.Equal(t, "b.md", result.Files[1].Name)
		assert.Equal(t, "content b\n", string(result.Files[1].Content))
	})

	t.Run("DELETE", func(t *testing.T) {
		input := "===DELETE: old-notes.md===\n"

		result, err := Parse(strings.NewReader(input))

		require.NoError(t, err)
		require.Len(t, result.Deletes, 1)
		assert.Equal(t, "old-notes.md", result.Deletes[0])
	})

	t.Run("PROPOSAL with slash path", func(t *testing.T) {
		input := "===PROPOSAL: global/rules/x.md===\nproposed content\n===END===\n"

		result, err := Parse(strings.NewReader(input))

		require.NoError(t, err)
		require.Len(t, result.Proposals, 1)
		assert.Equal(t, "global/rules/x.md", result.Proposals[0].Name)
		assert.Equal(t, "proposed content\n", string(result.Proposals[0].Content))
	})

	t.Run("mixed ops with chatter lines", func(t *testing.T) {
		input := "Sure, here are the changes:\n" +
			"===FILE: a.md===\n" +
			"content a\n" +
			"===END===\n" +
			"Some commentary in between.\n" +
			"===DELETE: b.md===\n" +
			"===PROPOSAL: c/d.md===\n" +
			"proposed\n" +
			"===END===\n" +
			"Done.\n"

		result, err := Parse(strings.NewReader(input))

		require.NoError(t, err)
		require.Len(t, result.Files, 1)
		assert.Equal(t, "a.md", result.Files[0].Name)
		require.Len(t, result.Deletes, 1)
		assert.Equal(t, "b.md", result.Deletes[0])
		require.Len(t, result.Proposals, 1)
		assert.Equal(t, "c/d.md", result.Proposals[0].Name)
	})

	t.Run("NOTHING alone", func(t *testing.T) {
		input := "  \nNOTHING\n  \n"

		result, err := Parse(strings.NewReader(input))

		require.NoError(t, err)
		assert.True(t, result.Nothing)
		assert.Empty(t, result.Files)
		assert.Empty(t, result.Deletes)
		assert.Empty(t, result.Proposals)
	})

	t.Run("NOTHING alongside ops", func(t *testing.T) {
		input := "NOTHING\n===FILE: a.md===\ncontent\n===END===\n"

		result, err := Parse(strings.NewReader(input))

		require.NoError(t, err)
		assert.False(t, result.Nothing)
		require.Len(t, result.Files, 1)
		assert.Equal(t, "a.md", result.Files[0].Name)
	})

	t.Run("unterminated FILE returns ErrUnterminatedBlock", func(t *testing.T) {
		input := "===FILE: a.md===\ncontent without end\n"

		_, err := Parse(strings.NewReader(input))

		require.Error(t, err)
		assert.ErrorIs(t, err, ErrUnterminatedBlock)
	})

	t.Run("empty name returns ErrEmptyOpName", func(t *testing.T) {
		input := "===FILE: ===\ncontent\n===END===\n"

		_, err := Parse(strings.NewReader(input))

		require.Error(t, err)
		assert.ErrorIs(t, err, ErrEmptyOpName)
	})

	t.Run("content verbatim preservation including blank lines", func(t *testing.T) {
		input := "===FILE: a.md===\nfirst line\n\nthird line\n===END===\n"

		result, err := Parse(strings.NewReader(input))

		require.NoError(t, err)
		require.Len(t, result.Files, 1)
		assert.Equal(t, "first line\n\nthird line\n", string(result.Files[0].Content))
	})
}
