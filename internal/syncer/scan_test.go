package syncer

import (
	"os"
	"path/filepath"
	"testing"

	"claude-memory-sync/internal/manifest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

func TestScanLocal(t *testing.T) {
	t.Run("includes CLAUDE.md, rules, skills, agents, and mapped project memory files", func(t *testing.T) {
		claudeDir := t.TempDir()
		writeFile(t, filepath.Join(claudeDir, "CLAUDE.md"), "global claude")
		writeFile(t, filepath.Join(claudeDir, "rules", "go.md"), "go rules")
		writeFile(t, filepath.Join(claudeDir, "skills", "pr-description", "SKILL.md"), "skill content")
		writeFile(t, filepath.Join(claudeDir, "agents", "implementer.md"), "agent content")
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
			"global/skills/pr-description/SKILL.md",
			"global/agents/implementer.md",
			"projects/HOME/memory/notes.md",
			"projects/HOME-my-project/memory/sub/a.md",
		}, gotPaths)

		assert.Equal(t, filepath.Join(claudeDir, "CLAUDE.md"), paths["global/CLAUDE.md"])
		assert.Equal(t, filepath.Join(claudeDir, "skills", "pr-description", "SKILL.md"), paths["global/skills/pr-description/SKILL.md"])
		assert.Equal(t, filepath.Join(claudeDir, "agents", "implementer.md"), paths["global/agents/implementer.md"])
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

func TestScanGlobalTree(t *testing.T) {
	t.Run("scans nested files under dir with correct namespace path and hash", func(t *testing.T) {
		claudeDir := t.TempDir()
		skillPath := filepath.Join(claudeDir, "skills", "pr-description", "SKILL.md")
		writeFile(t, skillPath, "skill content")

		result, paths, err := scanGlobalTree(claudeDir, "skills", "global/skills/")
		require.NoError(t, err)

		wantSum, err := manifest.HashFile(skillPath)
		require.NoError(t, err)

		require.Len(t, result, 1)
		assert.Equal(t, "global/skills/pr-description/SKILL.md", result[0].Path)
		assert.Equal(t, wantSum, result[0].SHA256)
		assert.Equal(t, skillPath, paths["global/skills/pr-description/SKILL.md"])
	})

	t.Run("returns nil for missing dir", func(t *testing.T) {
		claudeDir := t.TempDir()

		result, paths, err := scanGlobalTree(claudeDir, "skills", "global/skills/")
		require.NoError(t, err)
		assert.Nil(t, result)
		assert.Nil(t, paths)
	})

	t.Run("skips symlinked dir", func(t *testing.T) {
		claudeDir := t.TempDir()
		realDir := t.TempDir()
		writeFile(t, filepath.Join(realDir, "SKILL.md"), "skill content")
		require.NoError(t, os.Symlink(realDir, filepath.Join(claudeDir, "skills")))

		result, paths, err := scanGlobalTree(claudeDir, "skills", "global/skills/")
		require.NoError(t, err)
		assert.Nil(t, result)
		assert.Nil(t, paths)
	})
}
