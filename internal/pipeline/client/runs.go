package client

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"claude-memory-sync/internal/domain"
	"claude-memory-sync/internal/runs"
)

type CreateRunRequest struct {
	Flow    string       `json:"flow"`
	Trigger runs.Trigger `json:"trigger"`
}

type AddRunStepRequest struct {
	Step       string          `json:"step"`
	Status     runs.StepStatus `json:"status"`
	StartedAt  time.Time       `json:"started_at"`
	FinishedAt time.Time       `json:"finished_at"`
	Error      *string         `json:"error,omitempty"`
	OutputTail *string         `json:"output_tail,omitempty"`
}

type FinishRunRequest struct {
	Status  runs.Status     `json:"status"`
	Error   *string         `json:"error,omitempty"`
	Summary json.RawMessage `json:"summary,omitempty"`
}

type ListRunsParams struct {
	Flow   string
	Status *runs.Status
	Before *time.Time
	Limit  int
}

type interruptRunsRequest struct {
	Flow string `json:"flow,omitempty"`
}

type pruneRunsRequest struct {
	OlderThanDays int `json:"older_than_days"`
}

type countResponse struct {
	Count int `json:"count"`
}

type listRunsResponse struct {
	Runs []runs.Run `json:"runs"`
}

func runPath(id, suffix string) string {
	return "/v1/runs/" + url.PathEscape(id) + suffix
}

func (c *Client) CreateRun(ctx context.Context, req CreateRunRequest) (runs.Run, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return runs.Run{}, domain.Error(err, "encode create run request failed")
	}
	return c.runCall(ctx, "create_run", http.MethodPost, "/v1/runs", body)
}

func (c *Client) AddRunStep(ctx context.Context, runID string, req AddRunStepRequest) (runs.Step, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return runs.Step{}, domain.Error(err, "encode add run step request failed")
	}

	data, _, err := c.do(ctx, "add_run_step", http.MethodPost, runPath(runID, "/steps"), body)
	if err != nil {
		return runs.Step{}, err
	}

	var step runs.Step
	if err := json.Unmarshal(data, &step); err != nil {
		return runs.Step{}, domain.Error(err, "decode add run step response failed", slog.String("operation", "add_run_step"))
	}
	return step, nil
}

func (c *Client) FinishRun(ctx context.Context, runID string, req FinishRunRequest) (runs.Run, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return runs.Run{}, domain.Error(err, "encode finish run request failed")
	}
	return c.runCall(ctx, "finish_run", http.MethodPost, runPath(runID, "/finish"), body)
}

func (c *Client) InterruptRuns(ctx context.Context, flow string) (int, error) {
	body, err := json.Marshal(interruptRunsRequest{Flow: flow})
	if err != nil {
		return 0, domain.Error(err, "encode interrupt runs request failed")
	}
	return c.countCall(ctx, "interrupt_runs", "/v1/runs/interrupt", body)
}

func (c *Client) ListRuns(ctx context.Context, params ListRunsParams) ([]runs.Run, error) {
	query := url.Values{}
	if params.Flow != "" {
		query.Set("flow", params.Flow)
	}
	if params.Status != nil {
		query.Set("status", string(*params.Status))
	}
	if params.Before != nil {
		query.Set("before", params.Before.Format(time.RFC3339))
	}
	if params.Limit > 0 {
		query.Set("limit", strconv.Itoa(params.Limit))
	}

	data, _, err := c.do(ctx, "list_runs", http.MethodGet, "/v1/runs?"+query.Encode(), nil)
	if err != nil {
		return nil, err
	}

	var resp listRunsResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, domain.Error(err, "decode list runs response failed", slog.String("operation", "list_runs"))
	}
	if resp.Runs == nil {
		return []runs.Run{}, nil
	}
	return resp.Runs, nil
}

func (c *Client) GetRun(ctx context.Context, runID string) (runs.RunDetail, error) {
	data, _, err := c.do(ctx, "get_run", http.MethodGet, runPath(runID, ""), nil)
	if err != nil {
		return runs.RunDetail{}, err
	}

	var detail runs.RunDetail
	if err := json.Unmarshal(data, &detail); err != nil {
		return runs.RunDetail{}, domain.Error(err, "decode get run response failed", slog.String("operation", "get_run"))
	}
	return detail, nil
}

func (c *Client) PruneRuns(ctx context.Context, olderThanDays int) (int, error) {
	body, err := json.Marshal(pruneRunsRequest{OlderThanDays: olderThanDays})
	if err != nil {
		return 0, domain.Error(err, "encode prune runs request failed")
	}
	return c.countCall(ctx, "prune_runs", "/v1/runs/prune", body)
}

func (c *Client) runCall(ctx context.Context, operation, method, path string, body []byte) (runs.Run, error) {
	data, _, err := c.do(ctx, operation, method, path, body)
	if err != nil {
		return runs.Run{}, err
	}

	var run runs.Run
	if err := json.Unmarshal(data, &run); err != nil {
		return runs.Run{}, domain.Error(err, "decode run response failed", slog.String("operation", operation))
	}
	return run, nil
}

func (c *Client) countCall(ctx context.Context, operation, path string, body []byte) (int, error) {
	data, _, err := c.do(ctx, operation, http.MethodPost, path, body)
	if err != nil {
		return 0, err
	}

	var resp countResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return 0, domain.Error(err, "decode count response failed", slog.String("operation", operation))
	}
	return resp.Count, nil
}
