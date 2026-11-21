package footprint_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"claude-memory-sync/internal/domain"
	"claude-memory-sync/internal/footprint"
)

func write(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, rel)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

func TestBuild(t *testing.T) {
	now := time.Date(2026, 11, 21, 10, 0, 0, 0, time.UTC)

	t.Run("missing work dir returns not found", func(t *testing.T) {
		_, err := footprint.Build(filepath.Join(t.TempDir(), "nope"), now)
		require.ErrorIs(t, err, domain.ErrNotFound)
	})

	t.Run("empty work dir yields zero report with empty slices", func(t *testing.T) {
		got, err := footprint.Build(t.TempDir(), now)
		require.NoError(t, err)
		assert.Equal(t, now, got.GeneratedAt)
		assert.Equal(t, footprint.Limits{MemoryLines: 200, MemoryBytes: 25600, WarnPct: 80}, got.Limits)
		assert.NotNil(t, got.Projects)
		assert.Empty(t, got.Projects)
		assert.NotNil(t, got.Global.RulesScoped)
		assert.NotNil(t, got.Global.RulesUnconditional)
		assert.Equal(t, 0, got.Global.Skills)
		assert.Equal(t, 0, got.Global.ClaudeMDBytes)
	})

	t.Run("line count with trailing newline", func(t *testing.T) {
		root := t.TempDir()
		write(t, root, "projects/a/memory/MEMORY.md", "x\ny\nz\n")
		got, err := footprint.Build(root, now)
		require.NoError(t, err)
		require.Len(t, got.Projects, 1)
		assert.Equal(t, 3, got.Projects[0].MemoryLines)
		assert.Equal(t, 6, got.Projects[0].MemoryBytes)
	})

	t.Run("line count without trailing newline", func(t *testing.T) {
		root := t.TempDir()
		write(t, root, "projects/a/memory/MEMORY.md", "x\ny\nz")
		got, err := footprint.Build(root, now)
		require.NoError(t, err)
		assert.Equal(t, 3, got.Projects[0].MemoryLines)
	})

	t.Run("pct from lines", func(t *testing.T) {
		root := t.TempDir()
		write(t, root, "projects/a/memory/MEMORY.md", strings.Repeat("\n", 100))
		got, err := footprint.Build(root, now)
		require.NoError(t, err)
		assert.Equal(t, 50, got.Projects[0].PctOfLimit)
	})

	t.Run("pct from bytes", func(t *testing.T) {
		root := t.TempDir()
		write(t, root, "projects/a/memory/MEMORY.md", strings.Repeat("a", 12800))
		got, err := footprint.Build(root, now)
		require.NoError(t, err)
		assert.Equal(t, 1, got.Projects[0].MemoryLines)
		assert.Equal(t, 50, got.Projects[0].PctOfLimit)
	})

	t.Run("entries exclude MEMORY.md and non markdown", func(t *testing.T) {
		root := t.TempDir()
		write(t, root, "projects/a/memory/MEMORY.md", "x\n")
		write(t, root, "projects/a/memory/one.md", "1")
		write(t, root, "projects/a/memory/two.md", "2")
		write(t, root, "projects/a/memory/notes.txt", "3")
		write(t, root, "projects/a/memory/sub/three.md", "4")
		got, err := footprint.Build(root, now)
		require.NoError(t, err)
		assert.Equal(t, 2, got.Projects[0].Entries)
	})

	t.Run("projects without memory dir are skipped and missing MEMORY.md is zero", func(t *testing.T) {
		root := t.TempDir()
		write(t, root, "projects/nomem/other.md", "x")
		write(t, root, "projects/empty/memory/one.md", "x")
		got, err := footprint.Build(root, now)
		require.NoError(t, err)
		require.Len(t, got.Projects, 1)
		assert.Equal(t, "empty", got.Projects[0].Key)
		assert.Equal(t, 0, got.Projects[0].MemoryLines)
		assert.Equal(t, 1, got.Projects[0].Entries)
	})

	t.Run("projects sorted by pct desc then key asc", func(t *testing.T) {
		root := t.TempDir()
		write(t, root, "projects/b/memory/MEMORY.md", strings.Repeat("\n", 20))
		write(t, root, "projects/a/memory/MEMORY.md", strings.Repeat("\n", 20))
		write(t, root, "projects/c/memory/MEMORY.md", strings.Repeat("\n", 100))
		got, err := footprint.Build(root, now)
		require.NoError(t, err)
		require.Len(t, got.Projects, 3)
		assert.Equal(t, "c", got.Projects[0].Key)
		assert.Equal(t, "a", got.Projects[1].Key)
		assert.Equal(t, "b", got.Projects[2].Key)
	})

	t.Run("rules split into scoped and unconditional", func(t *testing.T) {
		root := t.TempDir()
		write(t, root, "global/CLAUDE.md", "12345")
		scoped := "---\npaths:\n  - src/**\n---\nbody\n"
		write(t, root, "global/rules/z-scoped.md", scoped)
		write(t, root, "global/rules/a-scoped.md", scoped)
		write(t, root, "global/rules/body-only.md", "---\nname: x\n---\npaths: in body\n")
		write(t, root, "global/rules/no-paths.md", "---\ndescription: d\n---\nbody\n")
		write(t, root, "global/rules/plain.md", "plain")
		write(t, root, "global/rules/unclosed.md", "---\npaths: x\n")
		write(t, root, "global/rules/ignored.txt", "x")
		got, err := footprint.Build(root, now)
		require.NoError(t, err)
		assert.Equal(t, 5, got.Global.ClaudeMDBytes)
		assert.Equal(t, []string{"a-scoped.md", "z-scoped.md"}, got.Global.RulesScoped)
		assert.Equal(t, 2*len(scoped), got.Global.RulesScopedBytes)
		assert.Equal(t, []string{"body-only.md", "no-paths.md", "plain.md", "unclosed.md"}, got.Global.RulesUnconditional)
		assert.Equal(t, len("---\nname: x\n---\npaths: in body\n")+len("---\ndescription: d\n---\nbody\n")+len("plain")+len("---\npaths: x\n"), got.Global.RulesUnconditionalBytes)
	})

	t.Run("skills count ignores dirs without SKILL.md", func(t *testing.T) {
		root := t.TempDir()
		write(t, root, "global/skills/one/SKILL.md", "x")
		write(t, root, "global/skills/two/SKILL.md", "x")
		write(t, root, "global/skills/three/readme.md", "x")
		write(t, root, "global/skills/file.md", "x")
		got, err := footprint.Build(root, now)
		require.NoError(t, err)
		assert.Equal(t, 2, got.Global.Skills)
	})

	t.Run("symlinks are skipped", func(t *testing.T) {
		root := t.TempDir()
		outside := t.TempDir()
		write(t, outside, "secret.md", "outside content")
		write(t, outside, "SKILL.md", "x")
		require.NoError(t, os.MkdirAll(filepath.Join(root, "global", "rules"), 0o755))
		require.NoError(t, os.MkdirAll(filepath.Join(root, "global", "skills"), 0o755))
		require.NoError(t, os.MkdirAll(filepath.Join(root, "projects", "p", "memory"), 0o755))
		require.NoError(t, os.Symlink(filepath.Join(outside, "secret.md"), filepath.Join(root, "global", "rules", "link.md")))
		require.NoError(t, os.Symlink(filepath.Join(outside, "secret.md"), filepath.Join(root, "global", "CLAUDE.md")))
		require.NoError(t, os.Symlink(outside, filepath.Join(root, "global", "skills", "linked")))
		require.NoError(t, os.Symlink(filepath.Join(outside, "secret.md"), filepath.Join(root, "projects", "p", "memory", "MEMORY.md")))
		require.NoError(t, os.Symlink(outside, filepath.Join(root, "projects", "linkedproj")))
		got, err := footprint.Build(root, now)
		require.NoError(t, err)
		assert.Empty(t, got.Global.RulesUnconditional)
		assert.Empty(t, got.Global.RulesScoped)
		assert.Equal(t, 0, got.Global.ClaudeMDBytes)
		assert.Equal(t, 0, got.Global.Skills)
		require.Len(t, got.Projects, 1)
		assert.Equal(t, 0, got.Projects[0].MemoryBytes)
	})
}
