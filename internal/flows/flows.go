package flows

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"sync"
	"time"

	"claude-memory-sync/internal/domain"
	"claude-memory-sync/internal/llm"
	"claude-memory-sync/internal/pipeline"
	"claude-memory-sync/internal/pipeline/client"
	"claude-memory-sync/internal/proposalapply"
	"claude-memory-sync/internal/runs"
)

type Name string

const (
	Intake        Name = "intake"
	DailySynth    Name = "daily-synth"
	Taskmine      Name = "taskmine"
	ProposalApply Name = "proposal-apply"
)

const (
	ParamSynthModel       = "synth_model"
	ParamStalenessEnabled = "staleness_enabled"
	ParamHotcacheMaxBytes = "hotcache_max_bytes"
	ParamFidelityModel    = "fidelity_model"

	maxOutputTail = 8000
)

func Names() []Name {
	return []Name{Intake, DailySynth, Taskmine, ProposalApply}
}

func ParseName(raw string) (Name, error) {
	for _, n := range Names() {
		if string(n) == raw {
			return n, nil
		}
	}
	return "", domain.Error(domain.ErrValidation, "unknown flow", slog.String("flow", raw))
}

//go:generate go tool moq -out flows_mock.go . LLM:MockLLM Recorder:MockRecorder Settings:MockSettings Pipeline:MockPipeline

type LLM interface {
	Run(ctx context.Context, req llm.Request) (string, error)
}

type StepResult struct {
	Step       string
	Status     runs.StepStatus
	StartedAt  time.Time
	FinishedAt time.Time
	Error      *string
	OutputTail *string
}

type Recorder interface {
	Start(ctx context.Context, flow Name, trigger runs.Trigger) (string, error)
	Step(ctx context.Context, runID string, step StepResult) error
	Finish(ctx context.Context, runID string, status runs.Status, errText *string, summary json.RawMessage) error
}

type Settings interface {
	Prompt(ctx context.Context, name string) (string, error)
	Param(ctx context.Context, key string) (string, error)
}

type Pipeline interface {
	proposalapply.Pipeline
	List(ctx context.Context, params client.ListParams) ([]pipeline.Proposal, error)
	Rejections(ctx context.Context, kind *pipeline.Kind) ([]pipeline.Rejection, error)
}

type Config struct {
	ReposDir string
	WorkDir  string
	Lock     string
	Worker   string
}

type Deps struct {
	LLM      LLM
	Recorder Recorder
	Settings Settings
	Pipeline Pipeline
}

type Runner struct {
	cfg     Config
	deps    Deps
	mu      sync.Mutex
	running map[Name]bool
}

func New(cfg Config, deps Deps) *Runner {
	return &Runner{cfg: cfg, deps: deps, running: make(map[Name]bool)}
}

func (r *Runner) Running(flow Name) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.running[flow]
}

func (r *Runner) Run(ctx context.Context, flow Name, trigger runs.Trigger) (string, error) {
	body, err := r.body(flow)
	if err != nil {
		return "", err
	}
	if !r.acquire(flow) {
		return "", domain.Error(domain.ErrConflict, "flow already running", slog.String("flow", string(flow)))
	}
	defer r.release(flow)

	runID, err := r.deps.Recorder.Start(ctx, flow, trigger)
	if err != nil {
		return "", domain.WrapError(err, slog.String("flow", string(flow)))
	}

	x := &run{runner: r, id: runID, summary: map[string]int{}}
	flowErr := body(ctx, x)
	if err := r.finish(ctx, x, flowErr); err != nil {
		return runID, err
	}
	return runID, domain.WrapError(flowErr, slog.String("flow", string(flow)), slog.String("run_id", runID))
}

func (r *Runner) body(flow Name) (func(context.Context, *run) error, error) {
	switch flow {
	case Intake:
		return r.intake, nil
	case DailySynth:
		return r.dailySynth, nil
	case Taskmine:
		return r.taskmine, nil
	case ProposalApply:
		return r.proposalApply, nil
	}
	return nil, domain.Error(domain.ErrValidation, "unknown flow", slog.String("flow", string(flow)))
}

func (r *Runner) acquire(flow Name) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.running[flow] {
		return false
	}
	r.running[flow] = true
	return true
}

func (r *Runner) release(flow Name) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.running, flow)
}

func (r *Runner) finish(ctx context.Context, x *run, flowErr error) error {
	summary, err := json.Marshal(x.summary)
	if err != nil {
		return domain.Error(err, "encode run summary failed")
	}

	status := runs.StatusSucceeded
	var errText *string
	switch {
	case flowErr != nil:
		status = runs.StatusFailed
		msg := domain.UserMessage(flowErr)
		errText = &msg
	case x.failed > 0:
		status = runs.StatusPartial
	}

	if err := r.deps.Recorder.Finish(ctx, x.id, status, errText, summary); err != nil {
		return domain.WrapError(err, slog.String("run_id", x.id))
	}
	return nil
}

type run struct {
	runner  *Runner
	id      string
	failed  int
	summary map[string]int
}

func (x *run) count(key string, n int) {
	x.summary[key] += n
}

func (x *run) step(ctx context.Context, name string, fn func(context.Context) (string, error)) error {
	started := time.Now()
	output, err := fn(ctx)
	result := StepResult{Step: name, Status: runs.StepSucceeded, StartedAt: started, FinishedAt: time.Now(), OutputTail: tail(output)}
	if err != nil {
		x.failed++
		result.Status = runs.StepFailed
		msg := domain.UserMessage(err)
		result.Error = &msg
	}
	if recErr := x.runner.deps.Recorder.Step(ctx, x.id, result); recErr != nil {
		return domain.WrapError(recErr, slog.String("step", name))
	}
	return err
}

func (x *run) mark(ctx context.Context, name string, status runs.StepStatus, reason string) error {
	now := time.Now()
	result := StepResult{Step: name, Status: status, StartedAt: now, FinishedAt: now, OutputTail: tail(reason)}
	if err := x.runner.deps.Recorder.Step(ctx, x.id, result); err != nil {
		return domain.WrapError(err, slog.String("step", name))
	}
	return nil
}

func (x *run) ask(ctx context.Context, promptName, modelParam, input string) (string, error) {
	prompt, err := x.runner.deps.Settings.Prompt(ctx, promptName)
	if err != nil {
		return "", domain.WrapError(err, slog.String("prompt", promptName))
	}
	model, err := x.runner.deps.Settings.Param(ctx, modelParam)
	if err != nil {
		return "", domain.WrapError(err, slog.String("param", modelParam))
	}
	return x.runner.deps.LLM.Run(ctx, llm.Request{Model: model, Prompt: prompt + "\n\n" + input})
}

func tail(output string) *string {
	output = strings.TrimSpace(output)
	if output == "" {
		return nil
	}
	if len(output) > maxOutputTail {
		output = strings.ToValidUTF8(output[len(output)-maxOutputTail:], "")
	}
	return &output
}
