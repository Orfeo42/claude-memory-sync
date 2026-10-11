package pipeline

import (
	"testing"

	"claude-memory-sync/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseKind(t *testing.T) {
	t.Run("accepts every known kind", func(t *testing.T) {
		got, err := ParseKind("skill")
		require.NoError(t, err)
		assert.Equal(t, KindSkill, got)

		got, err = ParseKind("rule")
		require.NoError(t, err)
		assert.Equal(t, KindRule, got)

		got, err = ParseKind("agent")
		require.NoError(t, err)
		assert.Equal(t, KindAgent, got)

		got, err = ParseKind("global")
		require.NoError(t, err)
		assert.Equal(t, KindGlobal, got)

		got, err = ParseKind("project-entry")
		require.NoError(t, err)
		assert.Equal(t, KindProjectEntry, got)

		got, err = ParseKind("bug")
		require.NoError(t, err)
		assert.Equal(t, KindBug, got)

		got, err = ParseKind("one-time-task")
		require.NoError(t, err)
		assert.Equal(t, KindOneTimeTask, got)

		got, err = ParseKind("code-change")
		require.NoError(t, err)
		assert.Equal(t, KindCodeChange, got)
	})

	t.Run("rejects unknown kind", func(t *testing.T) {
		_, err := ParseKind("bogus")
		require.ErrorIs(t, err, domain.ErrValidation)
	})
}

func TestKindOf(t *testing.T) {
	t.Run("skill", func(t *testing.T) {
		assert.Equal(t, KindSkill, KindOf("global/skills/foo/SKILL.md"))
	})

	t.Run("rule", func(t *testing.T) {
		assert.Equal(t, KindRule, KindOf("global/rules/go.md"))
	})

	t.Run("agent", func(t *testing.T) {
		assert.Equal(t, KindAgent, KindOf("global/agents/impl.md"))
	})

	t.Run("global", func(t *testing.T) {
		assert.Equal(t, KindGlobal, KindOf("global/CLAUDE.md"))
	})

	t.Run("project entry", func(t *testing.T) {
		assert.Equal(t, KindProjectEntry, KindOf("projects/p/memory/note.md"))
	})

	t.Run("bug prefix", func(t *testing.T) {
		assert.Equal(t, KindBug, KindOf("projects/p/memory/bug-x.md"))
	})

	t.Run("task prefix", func(t *testing.T) {
		assert.Equal(t, KindOneTimeTask, KindOf("projects/p/memory/task-x.md"))
	})

	t.Run("todo prefix", func(t *testing.T) {
		assert.Equal(t, KindCodeChange, KindOf("projects/p/memory/todo-x.md"))
	})
}

func TestGroupOf(t *testing.T) {
	t.Run("skill group is the skill directory", func(t *testing.T) {
		assert.Equal(t, "foo", GroupOf("global/skills/foo/SKILL.md"))
		assert.Equal(t, "foo", GroupOf("global/skills/foo/sub/x.md"))
	})

	t.Run("non skill group is the base name", func(t *testing.T) {
		assert.Equal(t, "go.md", GroupOf("global/rules/go.md"))
		assert.Equal(t, "note.md", GroupOf("projects/p/memory/note.md"))
	})

	t.Run("skill path without a file falls back to base name", func(t *testing.T) {
		assert.Equal(t, "foo", GroupOf("global/skills/foo"))
	})
}

func TestPathspecOf(t *testing.T) {
	t.Run("project target", func(t *testing.T) {
		assert.Equal(t, "projects/p", PathspecOf("projects/p/memory/note.md"))
	})

	t.Run("global target", func(t *testing.T) {
		assert.Equal(t, "global", PathspecOf("global/rules/go.md"))
	})
}
