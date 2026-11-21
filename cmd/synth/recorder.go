package main

import (
	"context"
	"encoding/json"

	"claude-memory-sync/internal/flows"
	"claude-memory-sync/internal/pipeline/client"
	"claude-memory-sync/internal/runs"
)

//go:generate go tool moq -out recorder_mock.go . runStore:mockRunStore
type runStore interface {
	CreateRun(ctx context.Context, req client.CreateRunRequest) (runs.Run, error)
	AddRunStep(ctx context.Context, runID string, req client.AddRunStepRequest) (runs.Step, error)
	FinishRun(ctx context.Context, runID string, req client.FinishRunRequest) (runs.Run, error)
}

type recorder struct {
	store runStore
}

func (r recorder) Start(ctx context.Context, flow flows.Name, trigger runs.Trigger) (string, error) {
	run, err := r.store.CreateRun(ctx, client.CreateRunRequest{Flow: string(flow), Trigger: trigger})
	if err != nil {
		return "", err
	}
	return run.ID, nil
}

func (r recorder) Step(ctx context.Context, runID string, step flows.StepResult) error {
	_, err := r.store.AddRunStep(ctx, runID, client.AddRunStepRequest{
		Step:       step.Step,
		Status:     step.Status,
		StartedAt:  step.StartedAt,
		FinishedAt: step.FinishedAt,
		Error:      step.Error,
		OutputTail: step.OutputTail,
	})
	return err
}

func (r recorder) Finish(ctx context.Context, runID string, status runs.Status, errText *string, summary json.RawMessage) error {
	_, err := r.store.FinishRun(ctx, runID, client.FinishRunRequest{Status: status, Error: errText, Summary: summary})
	return err
}
