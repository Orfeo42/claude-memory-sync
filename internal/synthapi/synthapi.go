package synthapi

import (
	"context"
	"errors"
	"net/http"
	"time"

	"claude-memory-sync/internal/domain"
	"claude-memory-sync/internal/flows"
	"claude-memory-sync/internal/footprint"
	"claude-memory-sync/internal/llm"
	"claude-memory-sync/internal/pipeline"
	"claude-memory-sync/internal/pipeline/client"
	"claude-memory-sync/internal/runs"
	"claude-memory-sync/internal/synthsettings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
)

const (
	apiTitle   = "synth"
	apiVersion = "1.0.0"
	apiPrefix  = "/api/"
)

//go:generate go tool moq -out synthapi_mock.go . Pipeline:MockPipeline Settings:MockSettings Flows:MockFlows Scheduler:MockScheduler LLM:MockLLM

type Pipeline interface {
	List(ctx context.Context, params client.ListParams) ([]pipeline.Proposal, error)
	Get(ctx context.Context, id string) (pipeline.ProposalDetail, error)
	Decide(ctx context.Context, id string, req client.DecideRequest) (pipeline.Proposal, error)
	Revise(ctx context.Context, id string, req client.ReviseRequest) (pipeline.Proposal, error)
	Retry(ctx context.Context, id, actor string) (pipeline.Proposal, error)
	ListRuns(ctx context.Context, params client.ListRunsParams) ([]runs.Run, error)
	GetRun(ctx context.Context, runID string) (runs.RunDetail, error)
}

type PromptSettings interface {
	Prompt(ctx context.Context, name string) (string, error)
	Prompts(ctx context.Context) ([]synthsettings.PromptInfo, error)
	GetPrompt(ctx context.Context, name string) (synthsettings.Prompt, error)
	SetPrompt(ctx context.Context, name, content string) (synthsettings.Prompt, error)
	ResetPrompt(ctx context.Context, name string) (synthsettings.Prompt, error)
	PromptHistory(ctx context.Context, name string, limit int) ([]synthsettings.PromptVersion, error)
}

type ParamSettings interface {
	Params(ctx context.Context) ([]synthsettings.Param, error)
	SetParam(ctx context.Context, key, value string) (synthsettings.Param, error)
	ResetParam(ctx context.Context, key string) (synthsettings.Param, error)
}

type ScheduleSettings interface {
	Schedule(ctx context.Context, flow flows.Name) (string, error)
	SetSchedule(ctx context.Context, flow flows.Name, cron string) (string, error)
	ResetSchedule(ctx context.Context, flow flows.Name) (string, error)
}

type TokenSettings interface {
	SetToken(ctx context.Context, token string) (synthsettings.TokenStatus, error)
	ClearToken(ctx context.Context) (synthsettings.TokenStatus, error)
	TokenStatus(ctx context.Context) (synthsettings.TokenStatus, error)
}

type Settings interface {
	PromptSettings
	ParamSettings
	ScheduleSettings
	TokenSettings
}

type Flows interface {
	Run(ctx context.Context, flow flows.Name, trigger runs.Trigger) (string, error)
	Running(flow flows.Name) bool
}

type Scheduler interface {
	Reload(ctx context.Context, flow flows.Name) error
}

type LLM interface {
	Run(ctx context.Context, req llm.Request) (string, error)
}

type FootprintFunc func(workDir string, now time.Time) (footprint.Report, error)

type Deps struct {
	Pipeline  Pipeline
	Settings  Settings
	Flows     Flows
	Scheduler Scheduler
	LLM       LLM
	Footprint FootprintFunc
	WorkDir   string
	WebDir    string
}

type server struct {
	Deps
}

type emptyInput struct{}

func New(deps Deps) http.Handler {
	mux := http.NewServeMux()

	config := huma.DefaultConfig(apiTitle, apiVersion)
	config.OpenAPIPath = "/api/openapi"
	config.DocsPath = ""
	config.SchemasPath = "/api/schemas"
	config.CreateHooks = nil
	humaAPI := humago.New(mux, config)

	s := &server{Deps: deps}
	s.registerProposals(humaAPI)
	s.registerFlows(humaAPI)
	s.registerRuns(humaAPI)
	s.registerPrompts(humaAPI)
	s.registerSettings(humaAPI)
	s.registerReport(humaAPI)

	mux.Handle(apiPrefix, http.NotFoundHandler())
	mux.Handle("/", spaHandler(deps.WebDir))

	return mux
}

func toHumaError(ctx context.Context, err error) error {
	status := domain.StatusCodeFromError(err)
	if errors.Is(err, domain.ErrValidation) {
		status = http.StatusUnprocessableEntity
	}

	switch {
	case errors.Is(err, domain.ErrUpstream):
		domain.LogWarn(ctx, "synth request failed upstream", err)
	case status >= http.StatusInternalServerError:
		domain.LogError(ctx, "synth request failed", err)
	}

	return huma.NewError(status, domain.UserMessage(err))
}
