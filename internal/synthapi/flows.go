package synthapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"claude-memory-sync/internal/domain"
	"claude-memory-sync/internal/flows"
	"claude-memory-sync/internal/pipeline/client"
	"claude-memory-sync/internal/runs"
	"claude-memory-sync/internal/synthsettings"

	"github.com/danielgtaylor/huma/v2"
)

const schedulePath = "/api/flows/{name}/schedule"

type flowBody struct {
	Name            string    `json:"name"`
	Schedule        string    `json:"schedule"`
	DefaultSchedule string    `json:"default_schedule"`
	Running         bool      `json:"running"`
	LastRun         *runs.Run `json:"last_run"`
}

type flowNameInput struct {
	Name string `path:"name"`
}

type scheduleInput struct {
	Name string `path:"name"`
	Body struct {
		Cron string `json:"cron" minLength:"1" maxLength:"100"`
	}
}

type flowsOutput struct {
	Body struct {
		Flows []flowBody `json:"flows"`
	}
}

type flowOutput struct {
	Body flowBody
}

type runStartedOutput struct {
	Body struct {
		Flow string `json:"flow"`
	}
}

func (s *server) registerFlows(api huma.API) {
	huma.Register(api, huma.Operation{OperationID: "list-flows", Method: http.MethodGet, Path: "/api/flows"}, s.listFlows)
	huma.Register(api, huma.Operation{OperationID: "run-flow", Method: http.MethodPost, Path: "/api/flows/{name}/run", DefaultStatus: 202}, s.runFlow)
	huma.Register(api, huma.Operation{OperationID: "put-schedule", Method: http.MethodPut, Path: schedulePath}, s.putSchedule)
	huma.Register(api, huma.Operation{OperationID: "reset-schedule", Method: http.MethodDelete, Path: schedulePath}, s.resetSchedule)
}

func parseFlow(raw string) (flows.Name, error) {
	flow, err := flows.ParseName(raw)
	if err != nil {
		return "", domain.Error(domain.ErrNotFound, "unknown flow", slog.String("flow", raw))
	}
	return flow, nil
}

func (s *server) flowView(ctx context.Context, flow flows.Name) (flowBody, error) {
	schedule, err := s.Settings.Schedule(ctx, flow)
	if err != nil {
		return flowBody{}, err
	}
	last, err := s.Pipeline.ListRuns(ctx, client.ListRunsParams{Flow: string(flow), Limit: 1})
	if err != nil {
		return flowBody{}, err
	}

	view := flowBody{
		Name:            string(flow),
		Schedule:        schedule,
		DefaultSchedule: synthsettings.DefaultSchedule(flow),
		Running:         s.Flows.Running(flow),
	}
	if len(last) > 0 {
		view.LastRun = &last[0]
	}
	return view, nil
}

func (s *server) launch(ctx context.Context, flow flows.Name, trigger runs.Trigger) {
	bg := context.WithoutCancel(ctx)
	go func() {
		_, err := s.Flows.Run(bg, flow, trigger)
		switch {
		case err == nil:
		case errors.Is(err, domain.ErrConflict):
			slog.InfoContext(bg, "flow already running", slog.String("flow", string(flow)))
		case errors.Is(err, domain.ErrUpstream):
			domain.LogWarn(bg, "flow run failed upstream", err, slog.String("flow", string(flow)))
		default:
			domain.LogError(bg, "flow run failed", err, slog.String("flow", string(flow)))
		}
	}()
}

func (s *server) listFlows(ctx context.Context, _ *emptyInput) (*flowsOutput, error) {
	out := &flowsOutput{}
	out.Body.Flows = []flowBody{}
	for _, flow := range flows.Names() {
		view, err := s.flowView(ctx, flow)
		if err != nil {
			return nil, toHumaError(ctx, err)
		}
		out.Body.Flows = append(out.Body.Flows, view)
	}
	return out, nil
}

func (s *server) runFlow(ctx context.Context, in *flowNameInput) (*runStartedOutput, error) {
	flow, err := parseFlow(in.Name)
	if err != nil {
		return nil, toHumaError(ctx, err)
	}
	if s.Flows.Running(flow) {
		return nil, toHumaError(ctx, domain.Error(domain.ErrConflict, "flow already running", slog.String("flow", in.Name)))
	}

	s.launch(ctx, flow, runs.TriggerManual)

	out := &runStartedOutput{}
	out.Body.Flow = string(flow)
	return out, nil
}

func (s *server) putSchedule(ctx context.Context, in *scheduleInput) (*flowOutput, error) {
	flow, err := parseFlow(in.Name)
	if err != nil {
		return nil, toHumaError(ctx, err)
	}
	if _, err := s.Settings.SetSchedule(ctx, flow, in.Body.Cron); err != nil {
		return nil, toHumaError(ctx, err)
	}
	return s.reloaded(ctx, flow)
}

func (s *server) resetSchedule(ctx context.Context, in *flowNameInput) (*flowOutput, error) {
	flow, err := parseFlow(in.Name)
	if err != nil {
		return nil, toHumaError(ctx, err)
	}
	if _, err := s.Settings.ResetSchedule(ctx, flow); err != nil {
		return nil, toHumaError(ctx, err)
	}
	return s.reloaded(ctx, flow)
}

func (s *server) reloaded(ctx context.Context, flow flows.Name) (*flowOutput, error) {
	if err := s.Scheduler.Reload(ctx, flow); err != nil {
		return nil, toHumaError(ctx, err)
	}
	view, err := s.flowView(ctx, flow)
	if err != nil {
		return nil, toHumaError(ctx, err)
	}
	return &flowOutput{Body: view}, nil
}
