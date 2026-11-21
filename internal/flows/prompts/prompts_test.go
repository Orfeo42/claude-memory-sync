package prompts_test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"claude-memory-sync/internal/domain"
	"claude-memory-sync/internal/flows/prompts"
)

func TestNames(t *testing.T) {
	t.Run("returns all prompts sorted", func(t *testing.T) {
		assert.Equal(t, []string{
			"condense-fidelity",
			"distill-scan",
			"distill-write",
			"hotcache-digest",
			"intake-judge",
			"proposal-chat",
			"synth-condense",
			"synth-staleness",
			"taskmine-cleanup",
			"taskmine-draft",
		}, prompts.Names())
	})
}

func TestDefault(t *testing.T) {
	t.Run("known name returns file content", func(t *testing.T) {
		want, err := os.ReadFile("intake-judge.md")
		require.NoError(t, err)

		got, err := prompts.Default("intake-judge")
		require.NoError(t, err)
		assert.Equal(t, string(want), got)
	})

	t.Run("unknown name returns not found", func(t *testing.T) {
		_, err := prompts.Default("nope")
		require.ErrorIs(t, err, domain.ErrNotFound)
	})
}
