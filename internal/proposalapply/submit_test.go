package proposalapply

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"claude-memory-sync/internal/ops"
	"claude-memory-sync/internal/pipeline"
)

func TestSubmit(t *testing.T) {
	t.Run("files are grouped and created via the api", func(t *testing.T) {
		fake, c := newFakePipeline(t)
		submitter := NewSubmitter(c, "daily-synth", "run-1")

		outcome, err := submitter.Submit(t.Context(), []pipeline.File{
			{TargetPath: "global/skills/foo/SKILL.md", Content: "skill\n"},
			{TargetPath: "global/skills/foo/ref.md", Content: "ref\n"},
			{TargetPath: "global/rules/r.md", Content: "rule\n"},
		})
		require.NoError(t, err)
		assert.Equal(t, []string{"new-foo", "new-r.md"}, outcome.Created)
		assert.Empty(t, outcome.Rejected)
		require.Len(t, fake.created, 2)
		assert.Len(t, fake.created[0].Files, 2)
		assert.Equal(t, "daily-synth", fake.created[0].SourceFlow)
		assert.Equal(t, "run-1", fake.created[0].SourceRun)
	})

	t.Run("409 becomes rejected op with api detail", func(t *testing.T) {
		fake, c := newFakePipeline(t)
		fake.createStatus = http.StatusConflict
		fake.createMessage = "open proposal exists"
		submitter := NewSubmitter(c, "f", "r")

		outcome, err := submitter.Submit(t.Context(), []pipeline.File{{TargetPath: "global/rules/r.md", Content: "rule\n"}})
		require.NoError(t, err)
		assert.Empty(t, outcome.Created)
		assert.Equal(t, []ops.RejectedOp{{Name: "r.md", Reason: "open proposal exists"}}, outcome.Rejected)
	})

	t.Run("non rejection error is returned", func(t *testing.T) {
		fake, c := newFakePipeline(t)
		fake.createStatus = http.StatusInternalServerError
		fake.createMessage = "boom"
		submitter := NewSubmitter(c, "f", "r")

		_, err := submitter.Submit(t.Context(), []pipeline.File{{TargetPath: "global/rules/r.md", Content: "rule\n"}})
		require.Error(t, err)
	})

	t.Run("no files makes no api call", func(t *testing.T) {
		fake, c := newFakePipeline(t)
		submitter := NewSubmitter(c, "f", "r")

		outcome, err := submitter.Submit(t.Context(), nil)
		require.NoError(t, err)
		assert.Empty(t, outcome.Created)
		assert.Empty(t, fake.created)
	})
}

func TestGroupFiles(t *testing.T) {
	t.Run("skill files share a group and keep first-seen order", func(t *testing.T) {
		groups := groupFiles([]pipeline.File{
			{TargetPath: "global/rules/a.md"},
			{TargetPath: "global/skills/s/SKILL.md"},
			{TargetPath: "global/skills/s/ref.md"},
		})

		require.Len(t, groups, 2)
		assert.Equal(t, "a.md", groups[0].Name)
		assert.Equal(t, "s", groups[1].Name)
		assert.Len(t, groups[1].Files, 2)
	})

	t.Run("no files yields no groups", func(t *testing.T) {
		assert.Empty(t, groupFiles(nil))
	})
}
