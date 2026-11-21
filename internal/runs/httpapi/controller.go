package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	pipelineapi "claude-memory-sync/internal/pipeline/httpapi"
	"claude-memory-sync/internal/runs"
)

const hoursPerDay = 24

type controller struct {
	svc *runs.Service
}

type runOutput struct {
	Body runs.Run
}

type stepOutput struct {
	Body runs.Step
}

type createInput struct {
	Body struct {
		Flow    string `json:"flow" minLength:"1" maxLength:"64" pattern:"^[a-z][a-z0-9-]*$"`
		Trigger string `json:"trigger" enum:"schedule,manual,event"`
	}
}

type addStepInput struct {
	ID   string `path:"id" minLength:"1"`
	Body struct {
		Step       string    `json:"step" minLength:"1"`
		Status     string    `json:"status" enum:"succeeded,failed,skipped,lossy"`
		StartedAt  time.Time `json:"started_at"`
		FinishedAt time.Time `json:"finished_at"`
		Error      *string   `json:"error,omitempty"`
		OutputTail *string   `json:"output_tail,omitempty" maxLength:"8000"`
	}
}

type finishInput struct {
	ID   string `path:"id" minLength:"1"`
	Body struct {
		Status  string          `json:"status" enum:"succeeded,failed,partial"`
		Error   *string         `json:"error,omitempty"`
		Summary json.RawMessage `json:"summary,omitempty" type:"object"`
	}
}

type interruptInput struct {
	Body struct {
		Flow string `json:"flow,omitempty" maxLength:"64"`
	}
}

type countOutput struct {
	Body struct {
		Count int `json:"count"`
	}
}

type listInput struct {
	Flow   string    `query:"flow" maxLength:"64"`
	Status string    `query:"status" enum:"running,succeeded,failed,partial,interrupted"`
	Limit  int       `query:"limit" minimum:"1" maximum:"200" default:"50"`
	Before time.Time `query:"before" format:"date-time"`
}

type listOutput struct {
	Body struct {
		Runs []runs.Run `json:"runs"`
	}
}

type getInput struct {
	ID string `path:"id" minLength:"1"`
}

type getOutput struct {
	Body runs.RunDetail
}

type pruneInput struct {
	Body struct {
		OlderThanDays int `json:"older_than_days,omitempty" minimum:"1" default:"90"`
	}
}

func Register(svc *runs.Service) pipelineapi.Registrar {
	c := &controller{svc: svc}
	return func(api huma.API) {
		huma.Register(api, pipelineapi.Operation("create-run", http.MethodPost, "/v1/runs", pipelineapi.RoleWriter, http.StatusCreated), c.create)
		huma.Register(api, pipelineapi.Operation("add-run-step", http.MethodPost, "/v1/runs/{id}/steps", pipelineapi.RoleWriter, http.StatusCreated), c.addStep)
		huma.Register(api, pipelineapi.Operation("finish-run", http.MethodPost, "/v1/runs/{id}/finish", pipelineapi.RoleWriter, http.StatusOK), c.finish)
		huma.Register(api, pipelineapi.Operation("interrupt-runs", http.MethodPost, "/v1/runs/interrupt", pipelineapi.RoleWriter, http.StatusOK), c.interrupt)
		huma.Register(api, pipelineapi.Operation("list-runs", http.MethodGet, "/v1/runs", pipelineapi.RoleWriter, http.StatusOK), c.list)
		huma.Register(api, pipelineapi.Operation("get-run", http.MethodGet, "/v1/runs/{id}", pipelineapi.RoleWriter, http.StatusOK), c.get)
		huma.Register(api, pipelineapi.Operation("prune-runs", http.MethodPost, "/v1/runs/prune", pipelineapi.RoleWriter, http.StatusOK), c.prune)
	}
}

func (c *controller) create(ctx context.Context, in *createInput) (*runOutput, error) {
	trigger, err := runs.ParseTrigger(in.Body.Trigger)
	if err != nil {
		return nil, pipelineapi.ToHumaError(ctx, err)
	}

	run, err := c.svc.CreateRun(ctx, in.Body.Flow, trigger)
	if err != nil {
		return nil, pipelineapi.ToHumaError(ctx, err)
	}
	return &runOutput{Body: run}, nil
}

func (c *controller) addStep(ctx context.Context, in *addStepInput) (*stepOutput, error) {
	status, err := runs.ParseStepStatus(in.Body.Status)
	if err != nil {
		return nil, pipelineapi.ToHumaError(ctx, err)
	}

	step, err := c.svc.AddStep(ctx, runs.NewStep{
		RunID:      in.ID,
		Step:       in.Body.Step,
		Status:     status,
		StartedAt:  in.Body.StartedAt,
		FinishedAt: in.Body.FinishedAt,
		Error:      in.Body.Error,
		OutputTail: in.Body.OutputTail,
	})
	if err != nil {
		return nil, pipelineapi.ToHumaError(ctx, err)
	}
	return &stepOutput{Body: step}, nil
}

func (c *controller) finish(ctx context.Context, in *finishInput) (*runOutput, error) {
	status, err := runs.ParseStatus(in.Body.Status)
	if err != nil {
		return nil, pipelineapi.ToHumaError(ctx, err)
	}

	run, err := c.svc.FinishRun(ctx, runs.Finish{
		ID:      in.ID,
		Status:  status,
		Error:   in.Body.Error,
		Summary: in.Body.Summary,
	})
	if err != nil {
		return nil, pipelineapi.ToHumaError(ctx, err)
	}
	return &runOutput{Body: run}, nil
}

func (c *controller) interrupt(ctx context.Context, in *interruptInput) (*countOutput, error) {
	var flow *string
	if in.Body.Flow != "" {
		flow = &in.Body.Flow
	}

	n, err := c.svc.InterruptRuns(ctx, flow)
	if err != nil {
		return nil, pipelineapi.ToHumaError(ctx, err)
	}
	out := &countOutput{}
	out.Body.Count = n
	return out, nil
}

func (c *controller) list(ctx context.Context, in *listInput) (*listOutput, error) {
	filter := runs.ListFilter{Limit: in.Limit}

	if in.Flow != "" {
		filter.Flow = &in.Flow
	}
	if in.Status != "" {
		status, err := runs.ParseStatus(in.Status)
		if err != nil {
			return nil, pipelineapi.ToHumaError(ctx, err)
		}
		filter.Status = &status
	}
	if !in.Before.IsZero() {
		filter.Before = &in.Before
	}

	list, err := c.svc.ListRuns(ctx, filter)
	if err != nil {
		return nil, pipelineapi.ToHumaError(ctx, err)
	}
	out := &listOutput{}
	out.Body.Runs = list
	if out.Body.Runs == nil {
		out.Body.Runs = []runs.Run{}
	}
	return out, nil
}

func (c *controller) get(ctx context.Context, in *getInput) (*getOutput, error) {
	detail, err := c.svc.GetRun(ctx, in.ID)
	if err != nil {
		return nil, pipelineapi.ToHumaError(ctx, err)
	}
	if detail.Steps == nil {
		detail.Steps = []runs.Step{}
	}
	return &getOutput{Body: detail}, nil
}

func (c *controller) prune(ctx context.Context, in *pruneInput) (*countOutput, error) {
	olderThan := time.Duration(in.Body.OlderThanDays) * hoursPerDay * time.Hour
	n, err := c.svc.PruneRuns(ctx, olderThan)
	if err != nil {
		return nil, pipelineapi.ToHumaError(ctx, err)
	}
	out := &countOutput{}
	out.Body.Count = n
	return out, nil
}
