package syncer

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

func TestScanLocal(t *testing.T) {
	t.Run("includes CLAUDE.md, rules, and mapped project memory files", func(t *testing.T) {
		claudeDir := t.TempDir()
		writeFile(t, filepath.Join(claudeDir, "CLAUDE.md"), "global claude")
		writeFile(t, filepath.Join(claudeDir, "rules", "go.md"), "go rules")
		writeFile(t, filepath.Join(claudeDir, "projects", "-home-orfeo42", "memory", "notes.md"), "home notes")
		writeFile(t, filepath.Join(claudeDir, "projects", "-home-orfeo42-my-project", "memory", "sub", "a.md"), "project a notes")

		result, paths, err := scanLocal(claudeDir, "-home-orfeo42")
		require.NoError(t, err)

		gotPaths := make([]string, 0, len(result))
		for _, e := range result {
			gotPaths = append(gotPaths, e.Path)
		}

		assert.ElementsMatch(t, []string{
			"global/CLAUDE.md",
			"global/rules/go.md",
			"projects/HOME/memory/notes.md",
			"projects/HOME-my-project/memory/sub/a.md",
		}, gotPaths)

		assert.Equal(t, filepath.Join(claudeDir, "CLAUDE.md"), paths["global/CLAUDE.md"])
		assert.Equal(t, filepath.Join(claudeDir, "projects", "-home-orfeo42-my-project", "memory", "sub", "a.md"), paths["projects/HOME-my-project/memory/sub/a.md"])
	})

	t.Run("excludes session jsonl files next to memory dir", func(t *testing.T) {
		claudeDir := t.TempDir()
		writeFile(t, filepath.Join(claudeDir, "projects", "-home-orfeo42", "memory", "notes.md"), "notes")
		writeFile(t, filepath.Join(claudeDir, "projects", "-home-orfeo42", "sessions", "abc.jsonl"), "session data")
		writeFile(t, filepath.Join(claudeDir, "projects", "-home-orfeo42", "history.jsonl"), "history data")

		result, _, err := scanLocal(claudeDir, "-home-orfeo42")
		require.NoError(t, err)

		gotPaths := make([]string, 0, len(result))
		for _, e := range result {
			gotPaths = append(gotPaths, e.Path)
		}

		assert.ElementsMatch(t, []string{"projects/HOME/memory/notes.md"}, gotPaths)
	})

	t.Run("skips project slugs not matching the configured prefix", func(t *testing.T) {
		claudeDir := t.TempDir()
		writeFile(t, filepath.Join(claudeDir, "projects", "C--Users-x-project", "memory", "notes.md"), "notes")

		result, _, err := scanLocal(claudeDir, "-home-orfeo42")
		require.NoError(t, err)
		assert.Empty(t, result)
	})

	t.Run("skips symlinked CLAUDE.md", func(t *testing.T) {
		claudeDir := t.TempDir()
		realFile := filepath.Join(t.TempDir(), "real-claude.md")
		writeFile(t, realFile, "real content")
		require.NoError(t, os.Symlink(realFile, filepath.Join(claudeDir, "CLAUDE.md")))

		result, _, err := scanLocal(claudeDir, "-home-orfeo42")
		require.NoError(t, err)
		assert.Empty(t, result)
	})

	t.Run("skips symlinked rule files", func(t *testing.T) {
		claudeDir := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(claudeDir, "rules"), 0o755))
		realFile := filepath.Join(t.TempDir(), "real-rule.md")
		writeFile(t, realFile, "rule content")
		require.NoError(t, os.Symlink(realFile, filepath.Join(claudeDir, "rules", "go.md")))

		result, _, err := scanLocal(claudeDir, "-home-orfeo42")
		require.NoError(t, err)
		assert.Empty(t, result)
	})

	t.Run("handles missing CLAUDE.md, rules, and projects gracefully", func(t *testing.T) {
		claudeDir := t.TempDir()

		result, paths, err := scanLocal(claudeDir, "-home-orfeo42")
		require.NoError(t, err)
		assert.Empty(t, result)
		assert.Empty(t, paths)
	})
}
