package ops

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"claude-memory-sync/internal/pipeline"
)

const validEntry = "---\n" +
	"name: foo\n" +
	"description: a test entry\n" +
	"metadata:\n" +
	"  type: fact\n" +
	"---\n" +
	"body\n"

func TestApply(t *testing.T) {
	t.Run("valid file op is applied and index rebuilt", func(t *testing.T) {
		memoryDir := t.TempDir()

		input := "===FILE: foo.md===\n" + validEntry + "===END===\n"

		result, err := Apply(context.Background(), ApplyOptions{MemoryDir: memoryDir, AllowDelete: false, ProposalsDir: ""}, strings.NewReader(input))
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

		result, err := Apply(context.Background(), ApplyOptions{MemoryDir: memoryDir, AllowDelete: false, ProposalsDir: ""}, strings.NewReader(input))
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

		result, err := Apply(context.Background(), ApplyOptions{MemoryDir: memoryDir, AllowDelete: false, ProposalsDir: ""}, strings.NewReader(input))
		require.NoError(t, err)
		assert.Empty(t, result.Applied)
		require.Len(t, result.Rejected, 1)
		assert.Equal(t, "bad.md", result.Rejected[0].Name)

		_, statErr := os.Stat(filepath.Join(memoryDir, "bad.md"))
		assert.True(t, os.IsNotExist(statErr))
	})

	t.Run("hidden character content is rejected", func(t *testing.T) {
		memoryDir := t.TempDir()

		input := "===FILE: hidden.md===\n" + strings.Replace(validEntry, "body", "bo\u202edy", 1) + "===END===\n"

		result, err := Apply(context.Background(), ApplyOptions{MemoryDir: memoryDir, AllowDelete: false, ProposalsDir: ""}, strings.NewReader(input))
		require.NoError(t, err)
		assert.Empty(t, result.Applied)
		require.Len(t, result.Rejected, 1)
		assert.Equal(t, "hidden.md", result.Rejected[0].Name)

		_, statErr := os.Stat(filepath.Join(memoryDir, "hidden.md"))
		assert.True(t, os.IsNotExist(statErr))
	})

	t.Run("delete without allow-delete is rejected", func(t *testing.T) {
		memoryDir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(memoryDir, "foo.md"), []byte(validEntry), 0o600))

		input := "===DELETE: foo.md===\n"

		result, err := Apply(context.Background(), ApplyOptions{MemoryDir: memoryDir, AllowDelete: false, ProposalsDir: ""}, strings.NewReader(input))
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

		result, err := Apply(context.Background(), ApplyOptions{MemoryDir: memoryDir, AllowDelete: true, ProposalsDir: ""}, strings.NewReader(input))
		require.NoError(t, err)
		assert.Equal(t, []string{"foo.md"}, result.Deleted)
		assert.Empty(t, result.Rejected)

		_, statErr := os.Stat(filepath.Join(memoryDir, "foo.md"))
		assert.True(t, os.IsNotExist(statErr))
	})

	t.Run("delete of missing entry is rejected", func(t *testing.T) {
		memoryDir := t.TempDir()

		input := "===DELETE: missing.md===\n"

		result, err := Apply(context.Background(), ApplyOptions{MemoryDir: memoryDir, AllowDelete: true, ProposalsDir: ""}, strings.NewReader(input))
		require.NoError(t, err)
		assert.Empty(t, result.Deleted)
		require.Len(t, result.Rejected, 1)
		assert.Equal(t, "no such entry", result.Rejected[0].Reason)
	})

	t.Run("proposal is flattened with sidecar path file", func(t *testing.T) {
		memoryDir := t.TempDir()
		proposalsDir := t.TempDir()

		input := "===PROPOSAL: global/rules/new-rule.md===\n" + validEntry + "===END===\n"

		result, err := Apply(context.Background(), ApplyOptions{MemoryDir: memoryDir, AllowDelete: false, ProposalsDir: proposalsDir}, strings.NewReader(input))
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

		result, err := Apply(context.Background(), ApplyOptions{MemoryDir: memoryDir, AllowDelete: false, ProposalsDir: ""}, strings.NewReader("NOTHING\n"))
		require.NoError(t, err)
		assert.True(t, result.Nothing)
		assert.Empty(t, result.Applied)
		assert.Empty(t, result.Deleted)
		assert.Empty(t, result.Proposals)
		assert.Empty(t, result.Rejected)
	})
}

func TestApplyWithSubmitter(t *testing.T) {
	t.Run("valid proposals are submitted and ids reported", func(t *testing.T) {
		submitter := &MockProposalSubmitter{
			SubmitFunc: func(_ context.Context, _ []pipeline.File) (SubmitOutcome, error) {
				return SubmitOutcome{Created: []string{"new-foo", "new-r.md"}}, nil
			},
		}
		input := "===PROPOSAL: global/skills/foo/SKILL.md===\nskill\n===END===\n" +
			"===PROPOSAL: global/rules/r.md===\nrule\n===END===\n"

		result, err := Apply(context.Background(), ApplyOptions{MemoryDir: t.TempDir(), Submitter: submitter}, strings.NewReader(input))
		require.NoError(t, err)
		assert.Equal(t, []string{"new-foo", "new-r.md"}, result.ProposalIDs)
		assert.Empty(t, result.Proposals)
		assert.Empty(t, result.Rejected)

		calls := submitter.SubmitCalls()
		require.Len(t, calls, 1)
		assert.Equal(t, []pipeline.File{
			{TargetPath: "global/skills/foo/SKILL.md", Content: "skill\n"},
			{TargetPath: "global/rules/r.md", Content: "rule\n"},
		}, calls[0].Files)
	})

	t.Run("submitter rejections are reported", func(t *testing.T) {
		submitter := &MockProposalSubmitter{
			SubmitFunc: func(_ context.Context, _ []pipeline.File) (SubmitOutcome, error) {
				return SubmitOutcome{Rejected: []RejectedOp{{Name: "r.md", Reason: "open proposal exists"}}}, nil
			},
		}
		input := "===PROPOSAL: global/rules/r.md===\nrule\n===END===\n"

		result, err := Apply(context.Background(), ApplyOptions{MemoryDir: t.TempDir(), Submitter: submitter}, strings.NewReader(input))
		require.NoError(t, err)
		assert.Empty(t, result.ProposalIDs)
		assert.Equal(t, []RejectedOp{{Name: "r.md", Reason: "open proposal exists"}}, result.Rejected)
	})

	t.Run("invalid path is rejected without submitting it", func(t *testing.T) {
		submitter := &MockProposalSubmitter{
			SubmitFunc: func(_ context.Context, _ []pipeline.File) (SubmitOutcome, error) {
				return SubmitOutcome{}, nil
			},
		}
		input := "===PROPOSAL: etc/passwd===\nx\n===END===\n"

		result, err := Apply(context.Background(), ApplyOptions{MemoryDir: t.TempDir(), Submitter: submitter}, strings.NewReader(input))
		require.NoError(t, err)
		require.Len(t, result.Rejected, 1)
		assert.Equal(t, ReasonInvalidProposalPath, result.Rejected[0].Reason)
		require.Len(t, submitter.SubmitCalls(), 1)
		assert.Empty(t, submitter.SubmitCalls()[0].Files)
	})

	t.Run("secret content is rejected without submitting it", func(t *testing.T) {
		submitter := &MockProposalSubmitter{
			SubmitFunc: func(_ context.Context, _ []pipeline.File) (SubmitOutcome, error) {
				return SubmitOutcome{}, nil
			},
		}
		input := "===PROPOSAL: global/rules/r.md===\ntoken ghp_abcdefghijklmnopqrstuvwxyz0123456789\n===END===\n"

		result, err := Apply(context.Background(), ApplyOptions{MemoryDir: t.TempDir(), Submitter: submitter}, strings.NewReader(input))
		require.NoError(t, err)
		require.Len(t, result.Rejected, 1)
		assert.Empty(t, submitter.SubmitCalls()[0].Files)
	})

	t.Run("hidden character content is rejected without submitting it", func(t *testing.T) {
		submitter := &MockProposalSubmitter{
			SubmitFunc: func(_ context.Context, _ []pipeline.File) (SubmitOutcome, error) {
				return SubmitOutcome{}, nil
			},
		}
		input := "===PROPOSAL: global/rules/r.md===\nrule\u200bhidden\n===END===\n"

		result, err := Apply(context.Background(), ApplyOptions{MemoryDir: t.TempDir(), Submitter: submitter}, strings.NewReader(input))
		require.NoError(t, err)
		require.Len(t, result.Rejected, 1)
		assert.Empty(t, submitter.SubmitCalls()[0].Files)
	})

	t.Run("submitter error is returned", func(t *testing.T) {
		submitter := &MockProposalSubmitter{
			SubmitFunc: func(_ context.Context, _ []pipeline.File) (SubmitOutcome, error) {
				return SubmitOutcome{}, errSubmitFailed
			},
		}
		input := "===PROPOSAL: global/rules/r.md===\nrule\n===END===\n"

		_, err := Apply(context.Background(), ApplyOptions{MemoryDir: t.TempDir(), Submitter: submitter}, strings.NewReader(input))
		require.ErrorIs(t, err, errSubmitFailed)
	})
}

type submitTestError string

func (e submitTestError) Error() string { return string(e) }

const errSubmitFailed submitTestError = "submit failed"
