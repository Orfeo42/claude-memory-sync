package flows

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"claude-memory-sync/internal/gitutil/gittest"
	"claude-memory-sync/internal/llm"
	"claude-memory-sync/internal/runs"
)

const intakeEntry = "---\n" +
	"name: foo\n" +
	"description: a test entry\n" +
	"metadata:\n" +
	"  type: fact\n" +
	"---\n" +
	"body\n"

type intakeFixture struct {
	runner   *Runner
	llm      *MockLLM
	recorder *MockRecorder
	reposDir string
	workDir  string
	canon    string
	client   string
}

func newIntakeFixture(t *testing.T, llmOutput string, llmErr error) *intakeFixture {
	t.Helper()

	base := t.TempDir()
	f := &intakeFixture{
		reposDir: filepath.Join(base, "repos"),
		workDir:  filepath.Join(base, "work"),
	}
	f.canon = filepath.Join(f.reposDir, "canonical.git")
	f.client = filepath.Join(f.reposDir, "clients", "c1.git")
	gittest.InitBareRepo(t, f.canon)
	gittest.CommitFiles(t, f.canon, map[string]string{"README.md": "seed"}, "seed", time.Now())
	gittest.InitBareRepo(t, f.client)

	f.llm = &MockLLM{RunFunc: func(_ context.Context, _ llm.Request) (string, error) {
		return llmOutput, llmErr
	}}
	f.recorder = &MockRecorder{
		StartFunc:  func(_ context.Context, _ Name, _ runs.Trigger) (string, error) { return "run-1", nil },
		StepFunc:   func(_ context.Context, _ string, _ StepResult) error { return nil },
		FinishFunc: func(_ context.Context, _ string, _ runs.Status, _ *string, _ json.RawMessage) error { return nil },
	}
	settings := &MockSettings{
		PromptFunc: func(_ context.Context, _ string) (string, error) { return "judge prompt", nil },
		ParamFunc:  func(_ context.Context, _ string) (string, error) { return "sonnet", nil },
	}
	f.runner = New(
		Config{ReposDir: f.reposDir, WorkDir: f.workDir, Lock: filepath.Join(base, "state", "canonical.lock")},
		Deps{LLM: f.llm, Recorder: f.recorder, Settings: settings},
	)
	return f
}

func (f *intakeFixture) stepNames() []string {
	names := make([]string, 0, len(f.recorder.StepCalls()))
	for _, call := range f.recorder.StepCalls() {
		names = append(names, call.Step.Step+":"+string(call.Step.Status))
	}
	return names
}

func (f *intakeFixture) finishStatus() runs.Status {
	calls := f.recorder.FinishCalls()
	return calls[len(calls)-1].Status
}

func TestRunnerIntake(t *testing.T) {
	ctx := context.Background()

	t.Run("no client changes makes no llm call and succeeds", func(t *testing.T) {
		f := newIntakeFixture(t, "", nil)

		_, err := f.runner.Run(ctx, Intake, runs.TriggerManual)

		require.NoError(t, err)
		assert.Empty(t, f.llm.RunCalls())
		assert.Equal(t, runs.StatusSucceeded, f.finishStatus())
		assert.Equal(t, []string{
			"ensure_work:succeeded", "scan:succeeded", "config_merge:succeeded",
			"project_keys:succeeded", "push:succeeded", "advance_markers:succeeded",
		}, f.stepNames())
	})

	t.Run("changed project is judged, committed, pushed and marker advanced", func(t *testing.T) {
		f := newIntakeFixture(t, "===FILE: foo.md===\n"+intakeEntry+"===END===\n", nil)
		head := gittest.CommitFiles(t, f.client, map[string]string{
			"projects/proj1/memory/foo.md": intakeEntry,
		}, "client change", time.Now())

		_, err := f.runner.Run(ctx, Intake, runs.TriggerManual)

		require.NoError(t, err)
		require.Len(t, f.llm.RunCalls(), 1)
		req := f.llm.RunCalls()[0].Req
		assert.Equal(t, "sonnet", req.Model)
		assert.True(t, strings.HasPrefix(req.Prompt, "judge prompt\n\n"))

		content, err := os.ReadFile(filepath.Join(f.workDir, "projects", "proj1", "memory", "foo.md"))
		require.NoError(t, err)
		assert.Equal(t, intakeEntry, string(content))

		log := gittest.RunGit(t, f.canon, nil, "log", "--format=%s", "main")
		assert.Contains(t, log, "intake: proj1")
		marker := strings.TrimSpace(gittest.RunGit(t, f.client, nil, "rev-parse", "refs/intake/last-processed"))
		assert.Equal(t, head, marker)
		assert.Equal(t, runs.StatusSucceeded, f.finishStatus())
		assert.Contains(t, f.stepNames(), "judge:proj1:succeeded")
	})

	t.Run("llm error aborts before push and marker advance", func(t *testing.T) {
		f := newIntakeFixture(t, "", assert.AnError)
		gittest.CommitFiles(t, f.client, map[string]string{
			"projects/proj1/memory/foo.md": intakeEntry,
		}, "client change", time.Now())

		_, err := f.runner.Run(ctx, Intake, runs.TriggerManual)

		require.Error(t, err)
		assert.Equal(t, runs.StatusFailed, f.finishStatus())
		assert.Equal(t, []string{
			"ensure_work:succeeded", "scan:succeeded", "config_merge:succeeded",
			"project_keys:succeeded", "judge:proj1:failed",
		}, f.stepNames())
		out := gittest.RunGit(t, f.client, nil, "for-each-ref", "refs/intake")
		assert.Empty(t, strings.TrimSpace(out))
	})
}
