package syncer

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunCycleUpSync(t *testing.T) {
	t.Run("first cycle pushes the whole whitelist", func(t *testing.T) {
		cfg, base := newTestConfig(t, "host-a", "-home-orfeo42")
		writeClaudeMD(t, cfg.ClaudeDir, "hello")

		agent := New(cfg)
		require.NoError(t, agent.RunCycle(context.Background()))

		clientDir := clientBareDir(base, "host-a")
		assert.Equal(t, 1, bareCommitCount(t, clientDir))
		assert.Contains(t, bareLsTree(t, clientDir), "global/CLAUDE.md")
	})

	t.Run("second unchanged cycle produces no new commit", func(t *testing.T) {
		cfg, base := newTestConfig(t, "host-a", "-home-orfeo42")
		writeClaudeMD(t, cfg.ClaudeDir, "hello")

		agent := New(cfg)
		require.NoError(t, agent.RunCycle(context.Background()))
		require.NoError(t, agent.RunCycle(context.Background()))

		clientDir := clientBareDir(base, "host-a")
		assert.Equal(t, 1, bareCommitCount(t, clientDir))
	})

	t.Run("editing local file produces a new commit with the changed path", func(t *testing.T) {
		cfg, base := newTestConfig(t, "host-a", "-home-orfeo42")
		writeClaudeMD(t, cfg.ClaudeDir, "hello")

		agent := New(cfg)
		require.NoError(t, agent.RunCycle(context.Background()))

		writeClaudeMD(t, cfg.ClaudeDir, "hello updated")
		require.NoError(t, agent.RunCycle(context.Background()))

		clientDir := clientBareDir(base, "host-a")
		assert.Equal(t, 2, bareCommitCount(t, clientDir))
		assert.Contains(t, bareHeadDiffPaths(t, clientDir), "global/CLAUDE.md")
	})

	t.Run("deleting local file removes it from client tree", func(t *testing.T) {
		cfg, base := newTestConfig(t, "host-a", "-home-orfeo42")
		writeClaudeMD(t, cfg.ClaudeDir, "hello")

		agent := New(cfg)
		require.NoError(t, agent.RunCycle(context.Background()))

		require.NoError(t, os.Remove(filepath.Join(cfg.ClaudeDir, "CLAUDE.md")))
		require.NoError(t, agent.RunCycle(context.Background()))

		clientDir := clientBareDir(base, "host-a")
		assert.NotContains(t, bareLsTree(t, clientDir), "global/CLAUDE.md")
	})
}

func TestRunCycleDownSync(t *testing.T) {
	t.Run("applies canonical file to mapped local path", func(t *testing.T) {
		cfg, base := newTestConfig(t, "host-a", "-home-orfeo42")

		seedBareFile(t, canonicalBareDir(base), "global/rules/example.md", "rule content")

		agent := New(cfg)
		require.NoError(t, agent.RunCycle(context.Background()))

		content, err := os.ReadFile(filepath.Join(cfg.ClaudeDir, "rules", "example.md"))
		require.NoError(t, err)
		assert.Equal(t, "rule content", string(content))
	})

	t.Run("propagates canonical delete to local", func(t *testing.T) {
		cfg, base := newTestConfig(t, "host-a", "-home-orfeo42")

		seedBareFile(t, canonicalBareDir(base), "global/rules/example.md", "rule content")

		agent := New(cfg)
		require.NoError(t, agent.RunCycle(context.Background()))

		localPath := filepath.Join(cfg.ClaudeDir, "rules", "example.md")
		require.FileExists(t, localPath)

		removeBareFile(t, canonicalBareDir(base), "global/rules/example.md")
		require.NoError(t, agent.RunCycle(context.Background()))

		_, statErr := os.Stat(localPath)
		assert.True(t, os.IsNotExist(statErr))
	})

	t.Run("local wins when both local and canonical changed", func(t *testing.T) {
		cfg, base := newTestConfig(t, "host-a", "-home-orfeo42")

		seedBareFile(t, canonicalBareDir(base), "global/rules/example.md", "rule content")

		agent := New(cfg)
		require.NoError(t, agent.RunCycle(context.Background()))

		localPath := filepath.Join(cfg.ClaudeDir, "rules", "example.md")
		require.NoError(t, os.WriteFile(localPath, []byte("local edit"), 0o644))
		seedBareFile(t, canonicalBareDir(base), "global/rules/example.md", "remote edit")

		require.NoError(t, agent.RunCycle(context.Background()))

		content, err := os.ReadFile(localPath)
		require.NoError(t, err)
		assert.Equal(t, "local edit", string(content))
	})
}

func TestRunCycleEmptyCanonical(t *testing.T) {
	t.Run("succeeds with nothing applied when canonical has no commits yet", func(t *testing.T) {
		cfg, _ := newTestConfig(t, "host-a", "-home-orfeo42")
		writeClaudeMD(t, cfg.ClaudeDir, "hello")

		agent := New(cfg)
		require.NoError(t, agent.RunCycle(context.Background()))

		entries, err := os.ReadDir(cfg.ClaudeDir)
		require.NoError(t, err)
		assert.Len(t, entries, 1)
		assert.Equal(t, "CLAUDE.md", entries[0].Name())
	})
}
