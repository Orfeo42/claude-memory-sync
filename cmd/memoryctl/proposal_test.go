package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApplyProposal(t *testing.T) {
	t.Run("valid rules path is written to work dir", func(t *testing.T) {
		workDir := t.TempDir()
		proposalsDir := t.TempDir()

		flattened := filepath.Join(proposalsDir, "global__rules__new-rule.md")
		require.NoError(t, os.WriteFile(flattened, []byte(validEntry), 0o600))
		require.NoError(t, os.WriteFile(flattened+".path", []byte("global/rules/new-rule.md"), 0o600))

		err := applyProposal(workDir, flattened)
		require.NoError(t, err)

		content, err := os.ReadFile(filepath.Join(workDir, "global", "rules", "new-rule.md"))
		require.NoError(t, err)
		assert.Equal(t, validEntry, string(content))
	})

	t.Run("path traversal in sidecar is rejected", func(t *testing.T) {
		workDir := t.TempDir()
		proposalsDir := t.TempDir()

		flattened := filepath.Join(proposalsDir, "evil.md")
		require.NoError(t, os.WriteFile(flattened, []byte(validEntry), 0o600))
		require.NoError(t, os.WriteFile(flattened+".path", []byte("global/../../../etc/passwd"), 0o600))

		err := applyProposal(workDir, flattened)
		require.Error(t, err)

		_, statErr := os.Stat(filepath.Join(workDir, "etc", "passwd"))
		assert.True(t, os.IsNotExist(statErr))
	})

	t.Run("disallowed location is rejected", func(t *testing.T) {
		workDir := t.TempDir()
		proposalsDir := t.TempDir()

		flattened := filepath.Join(proposalsDir, "outside.md")
		require.NoError(t, os.WriteFile(flattened, []byte(validEntry), 0o600))
		require.NoError(t, os.WriteFile(flattened+".path", []byte("projects/foo/memory/bar.md"), 0o600))

		err := applyProposal(workDir, flattened)
		require.Error(t, err)
	})
}
