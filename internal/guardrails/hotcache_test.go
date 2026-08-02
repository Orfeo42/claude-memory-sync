package guardrails_test

import (
	"os"
	"path/filepath"
	"testing"

	"claude-memory-sync/internal/guardrails"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReplaceHotBlock(t *testing.T) {
	t.Run("inserts markers when absent", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "MEMORY.md"), []byte("# Memory index\n\n- [a](a.md) — entry\n"), 0o644))

		err := guardrails.ReplaceHotBlock(dir, []byte("hot digest"), 1024)
		require.NoError(t, err)

		got, err := os.ReadFile(filepath.Join(dir, "MEMORY.md"))
		require.NoError(t, err)
		content := string(got)
		assert.Contains(t, content, "<!-- HOT:BEGIN -->\nhot digest\n<!-- HOT:END -->\n")
		assert.Contains(t, content, "- [a](a.md) — entry\n")
	})

	t.Run("replaces content when markers already present", func(t *testing.T) {
		dir := t.TempDir()
		existing := "# Memory index\n\n<!-- HOT:BEGIN -->\nold digest\n<!-- HOT:END -->\n\n- [a](a.md) — entry\n"
		require.NoError(t, os.WriteFile(filepath.Join(dir, "MEMORY.md"), []byte(existing), 0o644))

		err := guardrails.ReplaceHotBlock(dir, []byte("new digest"), 1024)
		require.NoError(t, err)

		got, err := os.ReadFile(filepath.Join(dir, "MEMORY.md"))
		require.NoError(t, err)
		content := string(got)
		assert.Contains(t, content, "<!-- HOT:BEGIN -->\nnew digest\n<!-- HOT:END -->\n")
		assert.NotContains(t, content, "old digest")
		assert.Contains(t, content, "- [a](a.md) — entry\n")
	})

	t.Run("rejects an oversize digest and leaves the file untouched", func(t *testing.T) {
		dir := t.TempDir()
		original := "# Memory index\n\n- [a](a.md) — entry\n"
		require.NoError(t, os.WriteFile(filepath.Join(dir, "MEMORY.md"), []byte(original), 0o644))

		err := guardrails.ReplaceHotBlock(dir, []byte("this digest is too big"), 4)
		require.Error(t, err)
		require.ErrorIs(t, err, guardrails.ErrDigestTooLarge)

		got, err := os.ReadFile(filepath.Join(dir, "MEMORY.md"))
		require.NoError(t, err)
		assert.Equal(t, original, string(got))
	})

	t.Run("creates MEMORY.md with the default heading when missing", func(t *testing.T) {
		dir := t.TempDir()

		err := guardrails.ReplaceHotBlock(dir, []byte("fresh digest"), 1024)
		require.NoError(t, err)

		got, err := os.ReadFile(filepath.Join(dir, "MEMORY.md"))
		require.NoError(t, err)
		content := string(got)
		assert.Contains(t, content, "# Memory index")
		assert.Contains(t, content, "<!-- HOT:BEGIN -->\nfresh digest\n<!-- HOT:END -->\n")
	})
}
