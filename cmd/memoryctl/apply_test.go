package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const validEntry = "---\n" +
	"name: foo\n" +
	"description: a test entry\n" +
	"metadata:\n" +
	"  type: fact\n" +
	"---\n" +
	"body\n"

func TestApplyOps(t *testing.T) {
	t.Run("valid file op is applied and index rebuilt", func(t *testing.T) {
		memoryDir := t.TempDir()

		input := "===FILE: foo.md===\n" + validEntry + "===END===\n"

		result, err := applyOps(memoryDir, false, "", strings.NewReader(input))
		require.NoError(t, err)
		assert.Equal(t, []string{"foo.md"}, result.Applied)
		assert.Empty(t, result.Rejected)

		content, err := os.ReadFile(filepath.Join(memoryDir, "foo.md"))
		require.NoError(t, err)
		assert.Contains(t, string(content), "body")

		index, err := os.ReadFile(filepath.Join(memoryDir, "MEMORY.md"))
		require.NoError(t, err)
		assert.Contains(t, string(index), "foo.md")
	})

	t.Run("missing frontmatter is rejected", func(t *testing.T) {
		memoryDir := t.TempDir()

		input := "===FILE: bad.md===\nno frontmatter here\n===END===\n"

		result, err := applyOps(memoryDir, false, "", strings.NewReader(input))
		require.NoError(t, err)
		assert.Empty(t, result.Applied)
		require.Len(t, result.Rejected, 1)
		assert.Equal(t, "bad.md", result.Rejected[0].Name)

		_, statErr := os.Stat(filepath.Join(memoryDir, "bad.md"))
		assert.True(t, os.IsNotExist(statErr))
	})

	t.Run("delete without allow-delete is rejected", func(t *testing.T) {
		memoryDir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(memoryDir, "foo.md"), []byte(validEntry), 0o600))

		input := "===DELETE: foo.md===\n"

		result, err := applyOps(memoryDir, false, "", strings.NewReader(input))
		require.NoError(t, err)
		assert.Empty(t, result.Deleted)
		require.Len(t, result.Rejected, 1)
		assert.Equal(t, "deletes not allowed", result.Rejected[0].Reason)

		_, statErr := os.Stat(filepath.Join(memoryDir, "foo.md"))
		assert.NoError(t, statErr)
	})

	t.Run("delete with allow-delete removes entry", func(t *testing.T) {
		memoryDir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(memoryDir, "foo.md"), []byte(validEntry), 0o600))

		input := "===DELETE: foo.md===\n"

		result, err := applyOps(memoryDir, true, "", strings.NewReader(input))
		require.NoError(t, err)
		assert.Equal(t, []string{"foo.md"}, result.Deleted)
		assert.Empty(t, result.Rejected)

		_, statErr := os.Stat(filepath.Join(memoryDir, "foo.md"))
		assert.True(t, os.IsNotExist(statErr))
	})

	t.Run("delete of missing entry is rejected", func(t *testing.T) {
		memoryDir := t.TempDir()

		input := "===DELETE: missing.md===\n"

		result, err := applyOps(memoryDir, true, "", strings.NewReader(input))
		require.NoError(t, err)
		assert.Empty(t, result.Deleted)
		require.Len(t, result.Rejected, 1)
		assert.Equal(t, "no such entry", result.Rejected[0].Reason)
	})

	t.Run("proposal is flattened with sidecar path file", func(t *testing.T) {
		memoryDir := t.TempDir()
		proposalsDir := t.TempDir()

		input := "===PROPOSAL: global/rules/new-rule.md===\n" + validEntry + "===END===\n"

		result, err := applyOps(memoryDir, false, proposalsDir, strings.NewReader(input))
		require.NoError(t, err)
		require.Len(t, result.Proposals, 1)
		assert.Equal(t, "global__rules__new-rule.md", result.Proposals[0])

		content, err := os.ReadFile(filepath.Join(proposalsDir, "global__rules__new-rule.md"))
		require.NoError(t, err)
		assert.Contains(t, string(content), "body")

		sidecar, err := os.ReadFile(filepath.Join(proposalsDir, "global__rules__new-rule.md.path"))
		require.NoError(t, err)
		assert.Equal(t, "global/rules/new-rule.md", string(sidecar))
	})

	t.Run("nothing marker yields empty result", func(t *testing.T) {
		memoryDir := t.TempDir()

		result, err := applyOps(memoryDir, false, "", strings.NewReader("NOTHING\n"))
		require.NoError(t, err)
		assert.True(t, result.Nothing)
		assert.Empty(t, result.Applied)
		assert.Empty(t, result.Deleted)
		assert.Empty(t, result.Proposals)
		assert.Empty(t, result.Rejected)
	})
}
