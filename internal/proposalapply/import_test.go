package proposalapply

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"claude-memory-sync/internal/domain"
	"claude-memory-sync/internal/ops"
	"claude-memory-sync/internal/pipeline"
	"claude-memory-sync/internal/pipeline/client"
)

func TestImport(t *testing.T) {
	t.Run("groups files by proposal and uses dir basename as source run", func(t *testing.T) {
		fake, c := newFakePipeline(t)
		dir := filepath.Join(t.TempDir(), "job-123")
		require.NoError(t, os.MkdirAll(dir, 0o750))
		writeStaged(t, dir, "global__skills__foo__SKILL.md", "global/skills/foo/SKILL.md", "skill\n")
		writeStaged(t, dir, "global__skills__foo__extra.md", "global/skills/foo/extra.md", "extra\n")
		writeStaged(t, dir, "global__rules__r.md", "global/rules/r.md", "rule\n")

		outcome, err := Import(t.Context(), c, dir, "taskmine")
		require.NoError(t, err)
		assert.Equal(t, []string{"new-r.md", "new-foo"}, outcome.Created)
		assert.Empty(t, outcome.Rejected)
		require.Len(t, fake.created, 2)
		for _, req := range fake.created {
			assert.Equal(t, "taskmine", req.SourceFlow)
			assert.Equal(t, "job-123", req.SourceRun)
		}
		assert.Len(t, fake.created[0].Files, 1)
		assert.Len(t, fake.created[1].Files, 2)
	})

	t.Run("409 becomes rejected entry", func(t *testing.T) {
		fake, c := newFakePipeline(t)
		fake.createStatus = http.StatusConflict
		fake.createMessage = "open proposal exists"
		dir := t.TempDir()
		writeStaged(t, dir, "global__rules__r.md", "global/rules/r.md", "rule\n")

		outcome, err := Import(t.Context(), c, dir, "taskmine")
		require.NoError(t, err)
		assert.Empty(t, outcome.Created)
		assert.Equal(t, []ops.RejectedOp{{Name: "r.md", Reason: "open proposal exists"}}, outcome.Rejected)
	})

	t.Run("invalid target is rejected without calling the api", func(t *testing.T) {
		fake, c := newFakePipeline(t)
		dir := t.TempDir()
		writeStaged(t, dir, "evil.md", "global/../evil.md", "x")

		outcome, err := Import(t.Context(), c, dir, "taskmine")
		require.NoError(t, err)
		assert.Empty(t, outcome.Created)
		require.Len(t, outcome.Rejected, 1)
		assert.Equal(t, "evil.md", outcome.Rejected[0].Name)
		assert.Empty(t, fake.created)
	})

	t.Run("missing directory fails", func(t *testing.T) {
		_, c := newFakePipeline(t)

		_, err := Import(t.Context(), c, filepath.Join(t.TempDir(), "nope"), "taskmine")
		require.Error(t, err)
	})
}

func TestImportRejections(t *testing.T) {
	t.Run("infers kinds dedupes and skips empty lines", func(t *testing.T) {
		fake, c := newFakePipeline(t)
		file := filepath.Join(t.TempDir(), "taskmine-rejected")
		content := "my-skill\n\n  bug-login.md \ntask-x.md\ntodo-y.md\nnotes.md\nmy-skill\n\n"
		require.NoError(t, os.WriteFile(file, []byte(content), 0o600))

		result, err := ImportRejections(t.Context(), c, ImportRejectionsOptions{File: file, Actor: "migrator"})
		require.NoError(t, err)
		assert.Equal(t, 5, result.Imported)

		reason := "imported from taskmine-rejected"
		assert.Equal(t, []client.AddRejectionRequest{
			{Kind: pipeline.KindSkill, Name: "my-skill", Actor: "migrator", Reason: &reason},
			{Kind: pipeline.KindBug, Name: "bug-login.md", Actor: "migrator", Reason: &reason},
			{Kind: pipeline.KindOneTimeTask, Name: "task-x.md", Actor: "migrator", Reason: &reason},
			{Kind: pipeline.KindCodeChange, Name: "todo-y.md", Actor: "migrator", Reason: &reason},
			{Kind: pipeline.KindProjectEntry, Name: "notes.md", Actor: "migrator", Reason: &reason},
		}, fake.rejections)
	})

	t.Run("missing file is a validation error", func(t *testing.T) {
		_, c := newFakePipeline(t)

		_, err := ImportRejections(t.Context(), c, ImportRejectionsOptions{Actor: "a"})
		require.ErrorIs(t, err, domain.ErrValidation)
	})

	t.Run("nonexistent file is a validation error", func(t *testing.T) {
		_, c := newFakePipeline(t)

		_, err := ImportRejections(t.Context(), c, ImportRejectionsOptions{File: filepath.Join(t.TempDir(), "nope"), Actor: "a"})
		require.ErrorIs(t, err, domain.ErrValidation)
	})
}
