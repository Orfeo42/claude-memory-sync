package flows

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
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

const (
	synthMergedOps = "===FILE: merged.md===\n---\nname: merged\ndescription: merged alpha\nmetadata:\n  type: fact\n---\nalpha merged\n===END===\n" +
		"===DELETE: a1.md===\n===DELETE: a2.md===\n"
	synthClusterLine = `{"kind":"cluster","title":"shared rule","entries":["proj-a/merged.md","proj-b/b1.md"],"reason":"r"}`
	synthProposalOps = "===PROPOSAL: global/rules/shared.md===\n---\nname: shared\ndescription: shared\nmetadata:\n  type: rule\n---\nbody\n===END===\n"
)

func synthEntry(name, word string) string {
	return "---\nname: " + name + "\ndescription: " + word + " entry\nmetadata:\n  type: fact\n---\n" + word + " body\n"
}

type synthFixture struct {
	runner   *Runner
	workDir  string
	bareDir  string
	steps    []StepResult
	status   runs.Status
	condense func(prompt string) (string, error)
	fidelity func(prompt string) (string, error)
	scan     string
	write    string
	created  []client.CreateRequest
}

func newSynthFixture(t *testing.T, betaEntries int) *synthFixture {
	t.Helper()

	f := &synthFixture{
		condense: func(prompt string) (string, error) {
			if strings.Contains(prompt, "alpha") {
				return synthMergedOps, nil
			}
			return "NOTHING", nil
		},
		fidelity: func(string) (string, error) { return `{"lost": []}`, nil },
		scan:     "NOTHING",
		write:    "NOTHING",
	}

	reposDir := t.TempDir()
	f.bareDir = filepath.Join(reposDir, "canonical.git")
	gittest.InitBareRepo(t, f.bareDir)
	seed := map[string]string{
		"projects/proj-a/memory/MEMORY.md": "# Memory index\n",
		"projects/proj-a/memory/a1.md":     synthEntry("a1", "alpha one"),
		"projects/proj-a/memory/a2.md":     synthEntry("a2", "alpha two"),
		"projects/proj-b/memory/MEMORY.md": "# Memory index\n",
	}
	names := []string{"b1", "b2", "b3"}
	for _, name := range names[:betaEntries] {
		seed["projects/proj-b/memory/"+name+".md"] = synthEntry(name, "beta "+name)
	}
	gittest.CommitFiles(t, f.bareDir, seed, "seed", time.Now())

	f.workDir = filepath.Join(t.TempDir(), "work")
	cfg := Config{ReposDir: reposDir, WorkDir: f.workDir, Lock: filepath.Join(t.TempDir(), "canonical.lock"), Worker: "w"}

	params := map[string]string{ParamSynthModel: "m", ParamFidelityModel: "f", ParamStalenessEnabled: "false", ParamHotcacheMaxBytes: "4000"}
	deps := Deps{
		LLM: &MockLLM{RunFunc: func(_ context.Context, req llm.Request) (string, error) {
			switch {
			case strings.HasPrefix(req.Prompt, "PROMPT:condense-fidelity"):
				return f.fidelity(req.Prompt)
			case strings.HasPrefix(req.Prompt, "PROMPT:synth-condense"):
				return f.condense(req.Prompt)
			case strings.HasPrefix(req.Prompt, "PROMPT:distill-scan"):
				return f.scan, nil
			case strings.HasPrefix(req.Prompt, "PROMPT:distill-write"):
				return f.write, nil
			default:
				return "digest line", nil
			}
		}},
		Recorder: &MockRecorder{
			StartFunc: func(context.Context, Name, runs.Trigger) (string, error) { return "run-1", nil },
			StepFunc: func(_ context.Context, _ string, step StepResult) error {
				f.steps = append(f.steps, step)
				return nil
			},
			FinishFunc: func(_ context.Context, _ string, status runs.Status, _ *string, _ json.RawMessage) error {
				f.status = status
				return nil
			},
		},
		Settings: &MockSettings{
			PromptFunc: func(_ context.Context, name string) (string, error) { return "PROMPT:" + name, nil },
			ParamFunc:  func(_ context.Context, key string) (string, error) { return params[key], nil },
		},
		Pipeline: &MockPipeline{CreateFunc: func(_ context.Context, req client.CreateRequest) (pipeline.Proposal, error) {
			f.created = append(f.created, req)
			return pipeline.Proposal{ID: "prop-1"}, nil
		}},
	}
	f.runner = New(cfg, deps)
	return f
}

func (f *synthFixture) step(t *testing.T, name string) StepResult {
	t.Helper()
	for _, s := range f.steps {
		if s.Step == name {
			return s
		}
	}
	require.Failf(t, "step not recorded", "step %q", name)
	return StepResult{}
}

func (f *synthFixture) bareLog(t *testing.T) string {
	t.Helper()
	return gittest.RunGit(t, f.bareDir, nil, "log", "--format=%s", "main")
}

func (f *synthFixture) fileExists(rel string) bool {
	_, err := os.Stat(filepath.Join(f.workDir, rel))
	return err == nil
}

func TestDailySynth(t *testing.T) {
	t.Run("condense applied when judge finds nothing lost", func(t *testing.T) {
		f := newSynthFixture(t, 1)

		_, err := f.runner.Run(t.Context(), DailySynth, runs.TriggerManual)
		require.NoError(t, err)

		assert.Equal(t, runs.StatusSucceeded, f.status)
		assert.True(t, f.fileExists("projects/proj-a/memory/merged.md"))
		assert.False(t, f.fileExists("projects/proj-a/memory/a1.md"))
		assert.Equal(t, runs.StepSucceeded, f.step(t, "condense:proj-a").Status)
		assert.Contains(t, f.bareLog(t), "synthesize: proj-a")
	})

	t.Run("condense NOTHING skips the fidelity judge", func(t *testing.T) {
		f := newSynthFixture(t, 1)
		f.condense = func(string) (string, error) { return "NOTHING\n", nil }
		judged := false
		f.fidelity = func(string) (string, error) {
			judged = true
			return `{"lost": []}`, nil
		}

		_, err := f.runner.Run(t.Context(), DailySynth, runs.TriggerManual)
		require.NoError(t, err)

		assert.False(t, judged)
		assert.Equal(t, runs.StepSucceeded, f.step(t, "condense:proj-a").Status)
		assert.True(t, f.fileExists("projects/proj-a/memory/a1.md"))
	})

	t.Run("lossy verdict marks step lossy and leaves files unchanged", func(t *testing.T) {
		f := newSynthFixture(t, 1)
		f.fidelity = func(string) (string, error) { return "```json\n{\"lost\": [\"the alpha port\"]}\n```", nil }

		_, err := f.runner.Run(t.Context(), DailySynth, runs.TriggerManual)
		require.NoError(t, err)

		step := f.step(t, "condense:proj-a")
		assert.Equal(t, runs.StepLossy, step.Status)
		require.NotNil(t, step.OutputTail)
		assert.Contains(t, *step.OutputTail, "the alpha port")
		assert.True(t, f.fileExists("projects/proj-a/memory/a1.md"))
		assert.False(t, f.fileExists("projects/proj-a/memory/merged.md"))
		assert.NotContains(t, f.bareLog(t), "synthesize: proj-a")
	})

	t.Run("judge garbage fails the step closed and leaves files unchanged", func(t *testing.T) {
		f := newSynthFixture(t, 1)
		f.fidelity = func(string) (string, error) { return "looks fine to me", nil }

		_, err := f.runner.Run(t.Context(), DailySynth, runs.TriggerManual)
		require.NoError(t, err)

		assert.Equal(t, runs.StepFailed, f.step(t, "condense:proj-a").Status)
		assert.Equal(t, runs.StatusPartial, f.status)
		assert.True(t, f.fileExists("projects/proj-a/memory/a1.md"))
		assert.False(t, f.fileExists("projects/proj-a/memory/merged.md"))
	})

	t.Run("judge error fails the step closed", func(t *testing.T) {
		f := newSynthFixture(t, 1)
		f.fidelity = func(string) (string, error) { return "", errors.New("judge down") }

		_, err := f.runner.Run(t.Context(), DailySynth, runs.TriggerManual)
		require.NoError(t, err)

		assert.Equal(t, runs.StepFailed, f.step(t, "condense:proj-a").Status)
		assert.False(t, f.fileExists("projects/proj-a/memory/merged.md"))
	})

	t.Run("one project LLM error does not stop the others", func(t *testing.T) {
		f := newSynthFixture(t, 2)
		f.condense = func(prompt string) (string, error) {
			if strings.Contains(prompt, "beta") {
				return "", errors.New("llm down")
			}
			return synthMergedOps, nil
		}

		_, err := f.runner.Run(t.Context(), DailySynth, runs.TriggerManual)
		require.NoError(t, err)

		assert.Equal(t, runs.StepFailed, f.step(t, "condense:proj-b").Status)
		assert.Equal(t, runs.StepSucceeded, f.step(t, "condense:proj-a").Status)
		assert.True(t, f.fileExists("projects/proj-a/memory/merged.md"))
		assert.Equal(t, runs.StatusPartial, f.status)
	})

	t.Run("distill proposals are submitted with source flow daily-synth", func(t *testing.T) {
		f := newSynthFixture(t, 1)
		f.scan = synthClusterLine
		f.write = synthProposalOps

		_, err := f.runner.Run(t.Context(), DailySynth, runs.TriggerManual)
		require.NoError(t, err)

		require.Len(t, f.created, 1)
		assert.Equal(t, "daily-synth", f.created[0].SourceFlow)
		assert.Equal(t, "run-1", f.created[0].SourceRun)
		require.Len(t, f.created[0].Files, 1)
		assert.Equal(t, "global/rules/shared.md", f.created[0].Files[0].TargetPath)
		assert.Equal(t, runs.StepSucceeded, f.step(t, "distill:shared rule").Status)
	})

	t.Run("unparseable cluster lines are skipped", func(t *testing.T) {
		f := newSynthFixture(t, 1)
		f.scan = "not json\n" + synthClusterLine
		f.write = synthProposalOps

		_, err := f.runner.Run(t.Context(), DailySynth, runs.TriggerManual)
		require.NoError(t, err)

		assert.Len(t, f.created, 1)
		assert.Equal(t, runs.StatusSucceeded, f.status)
	})

	t.Run("hotcache is written and committed for every project", func(t *testing.T) {
		f := newSynthFixture(t, 1)

		_, err := f.runner.Run(t.Context(), DailySynth, runs.TriggerManual)
		require.NoError(t, err)

		index, err := os.ReadFile(filepath.Join(f.workDir, "projects", "proj-b", "memory", "MEMORY.md"))
		require.NoError(t, err)
		assert.Contains(t, string(index), "digest line")
		log := f.bareLog(t)
		assert.Contains(t, log, "hotcache: proj-a")
		assert.Contains(t, log, "hotcache: proj-b")
	})

	t.Run("push step runs once at the end", func(t *testing.T) {
		f := newSynthFixture(t, 1)

		_, err := f.runner.Run(t.Context(), DailySynth, runs.TriggerManual)
		require.NoError(t, err)

		pushes := 0
		for _, s := range f.steps {
			if s.Step == "push" {
				pushes++
			}
		}
		assert.Equal(t, 1, pushes)
		assert.Equal(t, "push", f.steps[len(f.steps)-1].Step)
	})
}

func TestSynthParseLost(t *testing.T) {
	t.Run("empty list", func(t *testing.T) {
		lost, err := synthParseLost(" {\"lost\": []}\n")
		require.NoError(t, err)
		assert.Empty(t, lost)
	})

	t.Run("fenced json with facts", func(t *testing.T) {
		lost, err := synthParseLost("```json\n{\"lost\": [\"a\", \"b\"]}\n```")
		require.NoError(t, err)
		assert.Equal(t, []string{"a", "b"}, lost)
	})

	t.Run("prose around json is rejected", func(t *testing.T) {
		_, err := synthParseLost("Here: {\"lost\": []}")
		require.ErrorIs(t, err, errSynthJudgeFormat)
	})

	t.Run("missing lost key is rejected", func(t *testing.T) {
		_, err := synthParseLost("{}")
		require.ErrorIs(t, err, errSynthJudgeFormat)
	})

	t.Run("unknown field is rejected", func(t *testing.T) {
		_, err := synthParseLost(`{"lost": [], "extra": 1}`)
		require.ErrorIs(t, err, errSynthJudgeFormat)
	})

	t.Run("trailing data is rejected", func(t *testing.T) {
		_, err := synthParseLost(`{"lost": []} {"lost": []}`)
		require.ErrorIs(t, err, errSynthJudgeFormat)
	})
}

func TestSynthClusterLines(t *testing.T) {
	t.Run("NOTHING yields no clusters", func(t *testing.T) {
		assert.Empty(t, synthClusterLines(" NOTHING\n"))
	})

	t.Run("invalid lines are dropped", func(t *testing.T) {
		assert.Equal(t, []string{`{"a":1}`}, synthClusterLines("junk\n\n {\"a\":1} \n"))
	})
}
