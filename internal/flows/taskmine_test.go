package flows

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"claude-memory-sync/internal/gitutil/gittest"
	"claude-memory-sync/internal/llm"
	"claude-memory-sync/internal/pipeline"
	"claude-memory-sync/internal/pipeline/client"
	"claude-memory-sync/internal/runs"
)

const taskmineProposalOutput = "===PROPOSAL: global/skills/new-skill/SKILL.md===\n---\nname: new-skill\ndescription: Use when testing\n---\nbody\n===END===\n"

type taskmineFixture struct {
	runner   *Runner
	llm      *MockLLM
	recorder *MockRecorder
	pipeline *MockPipeline
}

func newTaskmineFixture(t *testing.T, llmOutput string, llmErr error) taskmineFixture {
	t.Helper()

	reposDir := filepath.Join(t.TempDir(), "repos")
	bare := filepath.Join(reposDir, "canonical.git")
	gittest.InitBareRepo(t, bare)
	gittest.CommitFiles(t, bare, map[string]string{
		"global/skills/old-skill/SKILL.md": "---\nname: old-skill\ndescription:   Use when old\n---\nbody\n",
		"projects/alpha/memory/MEMORY.md":  "- alpha hook\n",
		"projects/alpha/memory/entry.md":   "entry",
		"projects/beta/memory/other.md":    "no index",
	}, "seed", time.Now())

	f := taskmineFixture{
		llm: &MockLLM{RunFunc: func(_ context.Context, _ llm.Request) (string, error) {
			return llmOutput, llmErr
		}},
		recorder: &MockRecorder{
			StartFunc: func(_ context.Context, _ Name, _ runs.Trigger) (string, error) { return "run-1", nil },
			StepFunc:  func(_ context.Context, _ string, _ StepResult) error { return nil },
			FinishFunc: func(_ context.Context, _ string, _ runs.Status, _ *string, _ json.RawMessage) error {
				return nil
			},
		},
		pipeline: &MockPipeline{
			RejectionsFunc: func(_ context.Context, _ *pipeline.Kind) ([]pipeline.Rejection, error) {
				return []pipeline.Rejection{{Name: "rejected-b"}, {Name: "rejected-a"}, {Name: "rejected-a"}}, nil
			},
			ListFunc: func(_ context.Context, params client.ListParams) ([]pipeline.Proposal, error) {
				if *params.Status == pipeline.StatusPending {
					return []pipeline.Proposal{{Name: "pending-one"}}, nil
				}
				return []pipeline.Proposal{{Name: "approved-one"}}, nil
			},
			CreateFunc: func(_ context.Context, _ client.CreateRequest) (pipeline.Proposal, error) {
				return pipeline.Proposal{ID: "p1"}, nil
			},
		},
	}
	settings := &MockSettings{
		PromptFunc: func(_ context.Context, _ string) (string, error) { return "PROMPT", nil },
		ParamFunc:  func(_ context.Context, _ string) (string, error) { return "sonnet", nil },
	}
	f.runner = New(Config{
		ReposDir: reposDir,
		WorkDir:  filepath.Join(t.TempDir(), "work"),
		Lock:     filepath.Join(t.TempDir(), "canonical.lock"),
		Worker:   "w",
	}, Deps{LLM: f.llm, Recorder: f.recorder, Settings: settings, Pipeline: f.pipeline})
	return f
}

func TestRunnerTaskmine(t *testing.T) {
	t.Run("NOTHING creates no proposal and succeeds", func(t *testing.T) {
		f := newTaskmineFixture(t, "NOTHING", nil)

		runID, err := f.runner.Run(t.Context(), Taskmine, runs.TriggerManual)

		require.NoError(t, err)
		assert.Equal(t, "run-1", runID)
		assert.Empty(t, f.pipeline.CreateCalls())
		require.Len(t, f.recorder.FinishCalls(), 1)
		assert.Equal(t, runs.StatusSucceeded, f.recorder.FinishCalls()[0].Status)
		assert.Len(t, f.recorder.StepCalls(), 2)
	})

	t.Run("proposals are created with source flow and run id", func(t *testing.T) {
		f := newTaskmineFixture(t, taskmineProposalOutput, nil)

		_, err := f.runner.Run(t.Context(), Taskmine, runs.TriggerManual)

		require.NoError(t, err)
		require.Len(t, f.pipeline.CreateCalls(), 1)
		req := f.pipeline.CreateCalls()[0].Req
		assert.Equal(t, "taskmine", req.SourceFlow)
		assert.Equal(t, "run-1", req.SourceRun)
		require.Len(t, req.Files, 1)
		assert.Equal(t, "global/skills/new-skill/SKILL.md", req.Files[0].TargetPath)

		finish := f.recorder.FinishCalls()[0]
		assert.Equal(t, runs.StatusSucceeded, finish.Status)
		assert.JSONEq(t, `{"proposals":1,"rejected":2}`, string(finish.Summary))
	})

	t.Run("llm input carries skills, rejections, pending and indexes", func(t *testing.T) {
		f := newTaskmineFixture(t, "NOTHING", nil)

		_, err := f.runner.Run(t.Context(), Taskmine, runs.TriggerManual)

		require.NoError(t, err)
		require.Len(t, f.llm.RunCalls(), 1)
		want := "PROMPT\n\n" +
			"=== existing skills — patterns already covered, do NOT re-propose ===\n" +
			"- old-skill: Use when old\n\n" +
			"=== previously rejected proposals — do NOT re-propose ===\nrejected-a\nrejected-b\n\n" +
			"=== proposals currently pending human review — do NOT re-propose ===\napproved-one\npending-one\n\n" +
			"=== project: alpha ===\n- alpha hook\n\n"
		assert.Equal(t, want, f.llm.RunCalls()[0].Req.Prompt)
		assert.Equal(t, "sonnet", f.llm.RunCalls()[0].Req.Model)
	})

	t.Run("llm error fails the run", func(t *testing.T) {
		f := newTaskmineFixture(t, "", errors.New("boom"))

		_, err := f.runner.Run(t.Context(), Taskmine, runs.TriggerManual)

		require.Error(t, err)
		assert.Empty(t, f.pipeline.CreateCalls())
		require.Len(t, f.recorder.FinishCalls(), 1)
		assert.Equal(t, runs.StatusFailed, f.recorder.FinishCalls()[0].Status)
	})
}
