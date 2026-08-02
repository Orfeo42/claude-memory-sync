package guardrails_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"claude-memory-sync/internal/guardrails"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRebuildIndex(t *testing.T) {
	t.Run("preserves a custom header including embedded hot markers", func(t *testing.T) {
		dir := t.TempDir()
		header := "# Memory index\n\n<!-- HOT:BEGIN -->\ndigest text\n<!-- HOT:END -->\n\n"
		existing := header + "- [old](old.md) — stale entry\n"
		require.NoError(t, os.WriteFile(filepath.Join(dir, "MEMORY.md"), []byte(existing), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "aaa.md"), []byte("---\nname: A\ndescription: First entry\n---\n"), 0o644))

		err := guardrails.RebuildIndex(dir)
		require.NoError(t, err)

		got, err := os.ReadFile(filepath.Join(dir, "MEMORY.md"))
		require.NoError(t, err)
		assert.Contains(t, string(got), header)
		assert.Contains(t, string(got), "- [A](aaa.md) — First entry\n")
		assert.NotContains(t, string(got), "old.md")
	})

	t.Run("produces entries sorted by filename", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "zeta.md"), []byte("---\nname: Zeta\ndescription: Last\n---\n"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "alpha.md"), []byte("---\nname: Alpha\ndescription: First\n---\n"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "mid.md"), []byte("---\nname: Mid\ndescription: Middle\n---\n"), 0o644))

		err := guardrails.RebuildIndex(dir)
		require.NoError(t, err)

		got, err := os.ReadFile(filepath.Join(dir, "MEMORY.md"))
		require.NoError(t, err)

		alphaIdx := indexOfSubstring(t, string(got), "- [Alpha](alpha.md) — First\n")
		midIdx := indexOfSubstring(t, string(got), "- [Mid](mid.md) — Middle\n")
		zetaIdx := indexOfSubstring(t, string(got), "- [Zeta](zeta.md) — Last\n")

		assert.Less(t, alphaIdx, midIdx)
		assert.Less(t, midIdx, zetaIdx)
	})

	t.Run("preserves entire content when MEMORY.md has no bullet lines", func(t *testing.T) {
		dir := t.TempDir()
		existing := "# Notes\n\nSome freeform content.\nMore lines here.\n"
		require.NoError(t, os.WriteFile(filepath.Join(dir, "MEMORY.md"), []byte(existing), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "aaa.md"), []byte("---\nname: A\ndescription: First entry\n---\n"), 0o644))

		err := guardrails.RebuildIndex(dir)
		require.NoError(t, err)

		got, err := os.ReadFile(filepath.Join(dir, "MEMORY.md"))
		require.NoError(t, err)
		assert.True(t, strings.HasPrefix(string(got), existing))
		assert.Contains(t, string(got), "- [A](aaa.md) — First entry\n")
	})

	t.Run("uses the default header when MEMORY.md is missing", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "entry.md"), []byte("---\nname: Entry\ndescription: Something\n---\n"), 0o644))

		err := guardrails.RebuildIndex(dir)
		require.NoError(t, err)

		got, err := os.ReadFile(filepath.Join(dir, "MEMORY.md"))
		require.NoError(t, err)
		assert.Contains(t, string(got), "# Memory index\n\n")
		assert.Contains(t, string(got), "- [Entry](entry.md) — Something\n")
	})
}

func indexOfSubstring(t *testing.T, haystack, needle string) int {
	t.Helper()

	idx := -1
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			idx = i
			break
		}
	}

	require.NotEqual(t, -1, idx, "expected to find %q in %q", needle, haystack)
	return idx
}
