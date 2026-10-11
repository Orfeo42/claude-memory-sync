package main

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"claude-memory-sync/internal/domain"
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

		result, err := applyOps(context.Background(), memoryDir, false, "", nil, strings.NewReader(input))
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

	t.Run("missing memory dir is created", func(t *testing.T) {
		memoryDir := filepath.Join(t.TempDir(), "projects", "new", "memory")

		input := "===FILE: foo.md===\n" + validEntry + "===END===\n"

		result, err := applyOps(context.Background(), memoryDir, false, "", nil, strings.NewReader(input))
		require.NoError(t, err)
		assert.Equal(t, []string{"foo.md"}, result.Applied)

		_, err = os.Stat(filepath.Join(memoryDir, "foo.md"))
		require.NoError(t, err)

		index, err := os.ReadFile(filepath.Join(memoryDir, "MEMORY.md"))
		require.NoError(t, err)
		assert.Contains(t, string(index), "foo.md")
	})

	t.Run("missing frontmatter is rejected", func(t *testing.T) {
		memoryDir := t.TempDir()

		input := "===FILE: bad.md===\nno frontmatter here\n===END===\n"

		result, err := applyOps(context.Background(), memoryDir, false, "", nil, strings.NewReader(input))
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

		result, err := applyOps(context.Background(), memoryDir, false, "", nil, strings.NewReader(input))
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

		result, err := applyOps(context.Background(), memoryDir, true, "", nil, strings.NewReader(input))
		require.NoError(t, err)
		assert.Equal(t, []string{"foo.md"}, result.Deleted)
		assert.Empty(t, result.Rejected)

		_, statErr := os.Stat(filepath.Join(memoryDir, "foo.md"))
		assert.True(t, os.IsNotExist(statErr))
	})

	t.Run("delete of missing entry is rejected", func(t *testing.T) {
		memoryDir := t.TempDir()

		input := "===DELETE: missing.md===\n"

		result, err := applyOps(context.Background(), memoryDir, true, "", nil, strings.NewReader(input))
		require.NoError(t, err)
		assert.Empty(t, result.Deleted)
		require.Len(t, result.Rejected, 1)
		assert.Equal(t, "no such entry", result.Rejected[0].Reason)
	})

	t.Run("proposal is flattened with sidecar path file", func(t *testing.T) {
		memoryDir := t.TempDir()
		proposalsDir := t.TempDir()

		input := "===PROPOSAL: global/rules/new-rule.md===\n" + validEntry + "===END===\n"

		result, err := applyOps(context.Background(), memoryDir, false, proposalsDir, nil, strings.NewReader(input))
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

		result, err := applyOps(context.Background(), memoryDir, false, "", nil, strings.NewReader("NOTHING\n"))
		require.NoError(t, err)
		assert.True(t, result.Nothing)
		assert.Empty(t, result.Applied)
		assert.Empty(t, result.Deleted)
		assert.Empty(t, result.Proposals)
		assert.Empty(t, result.Rejected)
	})
}

func TestApplyOpsAPIMode(t *testing.T) {
	t.Run("proposals are grouped and created via the api", func(t *testing.T) {
		fake, c := newFakePipeline(t)
		submitter := &proposalSubmitter{client: c, sourceFlow: "daily-synth", sourceRun: "run-1"}

		input := "===PROPOSAL: global/skills/foo/SKILL.md===\nskill\n===END===\n" +
			"===PROPOSAL: global/skills/foo/ref.md===\nref\n===END===\n" +
			"===PROPOSAL: global/rules/r.md===\nrule\n===END===\n"

		result, err := applyOps(context.Background(), t.TempDir(), false, "", submitter, strings.NewReader(input))
		require.NoError(t, err)
		assert.Equal(t, []string{"new-foo", "new-r.md"}, result.ProposalIDs)
		assert.Empty(t, result.Proposals)
		assert.Empty(t, result.Rejected)
		require.Len(t, fake.created, 2)
		assert.Len(t, fake.created[0].Files, 2)
		assert.Equal(t, "daily-synth", fake.created[0].SourceFlow)
		assert.Equal(t, "run-1", fake.created[0].SourceRun)
	})

	t.Run("409 becomes rejected op with api detail", func(t *testing.T) {
		fake, c := newFakePipeline(t)
		fake.createStatus = http.StatusConflict
		fake.createMessage = "open proposal exists"
		submitter := &proposalSubmitter{client: c, sourceFlow: "f", sourceRun: "r"}

		input := "===PROPOSAL: global/rules/r.md===\nrule\n===END===\n"

		result, err := applyOps(context.Background(), t.TempDir(), false, "", submitter, strings.NewReader(input))
		require.NoError(t, err)
		assert.Empty(t, result.ProposalIDs)
		assert.Equal(t, []rejectedOp{{Name: "r.md", Reason: "open proposal exists"}}, result.Rejected)
	})

	t.Run("invalid path is rejected without calling the api", func(t *testing.T) {
		fake, c := newFakePipeline(t)
		submitter := &proposalSubmitter{client: c, sourceFlow: "f", sourceRun: "r"}

		input := "===PROPOSAL: etc/passwd===\nx\n===END===\n"

		result, err := applyOps(context.Background(), t.TempDir(), false, "", submitter, strings.NewReader(input))
		require.NoError(t, err)
		require.Len(t, result.Rejected, 1)
		assert.Equal(t, reasonInvalidProposalPath, result.Rejected[0].Reason)
		assert.Empty(t, fake.created)
	})
}

func TestCmdOpsApply(t *testing.T) {
	t.Run("source flags combined with proposals-dir are rejected", func(t *testing.T) {
		err := cmdOpsApply(context.Background(), []string{"--proposals-dir", "x", "--source-flow", "f", "--source-run", "r"})
		require.ErrorIs(t, err, domain.ErrValidation)
	})

	t.Run("source-flow without source-run is rejected", func(t *testing.T) {
		err := cmdOpsApply(context.Background(), []string{"--source-flow", "f"})
		require.ErrorIs(t, err, domain.ErrValidation)
	})
}
