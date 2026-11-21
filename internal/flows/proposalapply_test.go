package flows

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"claude-memory-sync/internal/gitutil/gittest"
	"claude-memory-sync/internal/llm"
	"claude-memory-sync/internal/pipeline"
	"claude-memory-sync/internal/runs"
)

func secretRuleProposal(id string) pipeline.Proposal {
	return pipeline.Proposal{
		ID:    id,
		Kind:  pipeline.KindRule,
		Name:  id + ".md",
		Files: []pipeline.File{{TargetPath: "global/rules/" + id + ".md", Content: "token ghp_abcdefghijklmnopqrstuvwxyz0123456789\n"}},
	}
}

func claimQueue(p *MockPipeline, proposals ...pipeline.Proposal) {
	queue := proposals
	p.ClaimFunc = func(context.Context, string, int) (pipeline.Proposal, error) {
		if len(queue) == 0 {
			return pipeline.Proposal{}, pipeline.ErrNothingToClaim
		}
		next := queue[0]
		queue = queue[1:]
		return next, nil
	}
	p.MarkAppliedFunc = func(_ context.Context, id, _, _ string) (pipeline.Proposal, error) {
		return pipeline.Proposal{ID: id}, nil
	}
	p.MarkFailedFunc = func(_ context.Context, id, _, _ string) (pipeline.Proposal, error) {
		return pipeline.Proposal{ID: id}, nil
	}
}

func newApplySettings() *MockSettings {
	return &MockSettings{
		PromptFunc: func(context.Context, string) (string, error) { return "CLEANUP PROMPT", nil },
		ParamFunc:  func(context.Context, string) (string, error) { return "sonnet", nil },
	}
}

func TestRunnerProposalApply(t *testing.T) {
	t.Run("nothing approved makes no llm call and succeeds", func(t *testing.T) {
		recorder, rec := newRecorder()
		llmMock := &MockLLM{}
		p := newIdlePipeline()
		r := New(newTestConfig(t, map[string]string{}), Deps{LLM: llmMock, Recorder: recorder, Settings: newApplySettings(), Pipeline: p})

		_, err := r.Run(t.Context(), ProposalApply, runs.TriggerManual)
		require.NoError(t, err)
		assert.Equal(t, runs.StatusSucceeded, rec.status)
		assert.Empty(t, llmMock.RunCalls())
		assert.Empty(t, p.MarkAppliedCalls())
		assert.Contains(t, *rec.stepNamed("expire").OutputTail, `"expired":0`)
		assert.Contains(t, *rec.stepNamed("apply").OutputTail, `"exit_code":0`)
		assert.Equal(t, "cleaned=0", *rec.stepNamed("cleanup").OutputTail)
		assert.Equal(t, "applied=0 failed=0", *rec.stepNamed("report").OutputTail)
	})

	t.Run("expire uses the proposal-apply actor and 336 hours", func(t *testing.T) {
		recorder, _ := newRecorder()
		p := newIdlePipeline()
		r := New(newTestConfig(t, map[string]string{}), Deps{LLM: &MockLLM{}, Recorder: recorder, Settings: newApplySettings(), Pipeline: p})

		_, err := r.Run(t.Context(), ProposalApply, runs.TriggerManual)
		require.NoError(t, err)
		calls := p.ExpireCalls()
		require.Len(t, calls, 1)
		assert.Equal(t, "proposal-apply", calls[0].Actor)
		assert.Equal(t, 336, calls[0].OlderThanHours)
	})

	t.Run("approved memory proposal is applied pushed and marked applied", func(t *testing.T) {
		recorder, rec := newRecorder()
		cfg := newTestConfig(t, map[string]string{})
		p := newIdlePipeline()
		claimQueue(p, pipeline.Proposal{
			ID:    "p1",
			Kind:  pipeline.KindRule,
			Name:  "one.md",
			Files: []pipeline.File{{TargetPath: "global/rules/one.md", Content: "one\n"}},
		})
		llmMock := &MockLLM{}
		r := New(cfg, Deps{LLM: llmMock, Recorder: recorder, Settings: newApplySettings(), Pipeline: p})

		_, err := r.Run(t.Context(), ProposalApply, runs.TriggerManual)
		require.NoError(t, err)
		assert.Equal(t, runs.StatusSucceeded, rec.status)
		assert.JSONEq(t, `{"applied":1,"failed":0}`, string(rec.summary))
		assert.Empty(t, llmMock.RunCalls())

		marks := p.MarkAppliedCalls()
		require.Len(t, marks, 1)
		assert.Equal(t, "p1", marks[0].ID)
		assert.Equal(t, "w1:run-1", marks[0].Actor)

		bare := filepath.Join(cfg.ReposDir, "canonical.git")
		head := strings.TrimSpace(gittest.RunGit(t, cfg.WorkDir, nil, "rev-parse", "HEAD"))
		assert.Equal(t, head, strings.TrimSpace(gittest.RunGit(t, bare, nil, "rev-parse", "main")))
		assert.Contains(t, gittest.RunGit(t, bare, nil, "log", "--format=%s", "main"), "proposal: apply rule one.md")
	})

	t.Run("approved skill proposal triggers the cleanup llm call and commits", func(t *testing.T) {
		recorder, rec := newRecorder()
		cfg := newTestConfig(t, map[string]string{
			"projects/proj/memory/entry.md": "---\nname: entry\ndescription: d\ntype: feedback\n---\nbody\n",
			"projects/proj/memory/keep.md":  "---\nname: keep\ndescription: d\ntype: feedback\n---\nkeep\n",
		})
		p := newIdlePipeline()
		claimQueue(p, pipeline.Proposal{
			ID:   "s1",
			Kind: pipeline.KindSkill,
			Name: "foo",
			Files: []pipeline.File{{
				TargetPath: "global/skills/foo/SKILL.md",
				Content:    "# foo\n\n## Evidence\n- proj/entry.md\n- proj/missing.md\n\n## Other\nproj/keep.md\n",
			}},
		})
		var gotRequest llm.Request
		llmMock := &MockLLM{RunFunc: func(_ context.Context, req llm.Request) (string, error) {
			gotRequest = req
			return "===DELETE: entry.md===\n", nil
		}}
		r := New(cfg, Deps{LLM: llmMock, Recorder: recorder, Settings: newApplySettings(), Pipeline: p})

		_, err := r.Run(t.Context(), ProposalApply, runs.TriggerManual)
		require.NoError(t, err)
		assert.Equal(t, runs.StatusSucceeded, rec.status)

		require.Len(t, llmMock.RunCalls(), 1)
		assert.Equal(t, "sonnet", gotRequest.Model)
		assert.True(t, strings.HasPrefix(gotRequest.Prompt, "CLEANUP PROMPT\n\n=== new skill: foo ===\n"))
		assert.Contains(t, gotRequest.Prompt, "=== candidate entries (project: proj) ===\n===ENTRY: entry.md===\n")
		assert.NotContains(t, gotRequest.Prompt, "===ENTRY: missing.md===")
		assert.NotContains(t, gotRequest.Prompt, "===ENTRY: keep.md===")
		assert.Equal(t, "cleaned=1", *rec.stepNamed("cleanup").OutputTail)

		_, statErr := os.Stat(filepath.Join(cfg.WorkDir, "projects", "proj", "memory", "entry.md"))
		assert.True(t, os.IsNotExist(statErr))

		bare := filepath.Join(cfg.ReposDir, "canonical.git")
		assert.Contains(t, gittest.RunGit(t, bare, nil, "log", "--format=%s", "main"), "taskmine: cleanup proj after foo")
		tree := gittest.RunGit(t, bare, nil, "ls-tree", "-r", "--name-only", "main")
		assert.NotContains(t, tree, "projects/proj/memory/entry.md")
		assert.Contains(t, tree, "projects/proj/memory/keep.md")
	})

	t.Run("cleanup llm failure fails the run", func(t *testing.T) {
		recorder, rec := newRecorder()
		cfg := newTestConfig(t, map[string]string{
			"projects/proj/memory/entry.md": "---\nname: entry\ndescription: d\ntype: feedback\n---\nbody\n",
		})
		p := newIdlePipeline()
		claimQueue(p, pipeline.Proposal{
			ID:    "s1",
			Kind:  pipeline.KindSkill,
			Name:  "foo",
			Files: []pipeline.File{{TargetPath: "global/skills/foo/SKILL.md", Content: "# foo\n\n## Evidence\n- proj/entry.md\n"}},
		})
		llmMock := &MockLLM{RunFunc: func(context.Context, llm.Request) (string, error) { return "", errCleanupLLM }}
		r := New(cfg, Deps{LLM: llmMock, Recorder: recorder, Settings: newApplySettings(), Pipeline: p})

		_, err := r.Run(t.Context(), ProposalApply, runs.TriggerManual)
		require.Error(t, err)
		assert.Equal(t, runs.StatusFailed, rec.status)
		assert.Equal(t, runs.StepFailed, rec.stepNamed("cleanup").Status)
		assert.Equal(t, runs.StepSucceeded, rec.stepNamed("apply").Status)
	})

	t.Run("partial apply failure finishes partial and lists the failure", func(t *testing.T) {
		recorder, rec := newRecorder()
		cfg := newTestConfig(t, map[string]string{})
		p := newIdlePipeline()
		claimQueue(p, secretRuleProposal("bad"), pipeline.Proposal{
			ID:    "good",
			Kind:  pipeline.KindRule,
			Name:  "good.md",
			Files: []pipeline.File{{TargetPath: "global/rules/good.md", Content: "ok\n"}},
		})
		r := New(cfg, Deps{LLM: &MockLLM{}, Recorder: recorder, Settings: newApplySettings(), Pipeline: p})

		_, err := r.Run(t.Context(), ProposalApply, runs.TriggerManual)
		require.NoError(t, err)
		assert.Equal(t, runs.StatusPartial, rec.status)
		assert.Nil(t, rec.errText)
		assert.JSONEq(t, `{"applied":1,"failed":1}`, string(rec.summary))

		applyStep := rec.stepNamed("apply")
		assert.Equal(t, runs.StepSucceeded, applyStep.Status)
		assert.Contains(t, *applyStep.OutputTail, `"failed":["bad"]`)
		assert.Contains(t, *applyStep.OutputTail, `"exit_code":1`)
		assert.Contains(t, *applyStep.OutputTail, `"error":`)

		reportStep := rec.stepNamed("report")
		assert.Equal(t, runs.StepFailed, reportStep.Status)
		assert.Equal(t, "applied=1 failed=1", *reportStep.OutputTail)
		require.Len(t, p.MarkFailedCalls(), 1)
		require.Len(t, p.MarkAppliedCalls(), 1)
	})
}

const errCleanupLLM = testError("llm down")

type testError string

func (e testError) Error() string { return string(e) }
