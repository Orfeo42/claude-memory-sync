package main

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"claude-memory-sync/internal/domain"
	"claude-memory-sync/internal/flows"
	"claude-memory-sync/internal/pipeline/client"
	"claude-memory-sync/internal/runs"
)

func TestRecorderStart(t *testing.T) {
	t.Run("creates the run and returns its id", func(t *testing.T) {
		store := &mockRunStore{CreateRunFunc: func(context.Context, client.CreateRunRequest) (runs.Run, error) {
			return runs.Run{ID: "run-1"}, nil
		}}

		id, err := recorder{store: store}.Start(t.Context(), flows.Intake, runs.TriggerSchedule)

		require.NoError(t, err)
		assert.Equal(t, "run-1", id)
		assert.Equal(t, client.CreateRunRequest{Flow: "intake", Trigger: runs.TriggerSchedule}, store.CreateRunCalls()[0].Req)
	})

	t.Run("store error is returned", func(t *testing.T) {
		store := &mockRunStore{CreateRunFunc: func(context.Context, client.CreateRunRequest) (runs.Run, error) {
			return runs.Run{}, domain.ErrUpstream
		}}

		_, err := recorder{store: store}.Start(t.Context(), flows.Intake, runs.TriggerManual)

		require.ErrorIs(t, err, domain.ErrUpstream)
	})
}

func TestRecorderStep(t *testing.T) {
	t.Run("maps every step field", func(t *testing.T) {
		store := &mockRunStore{AddRunStepFunc: func(context.Context, string, client.AddRunStepRequest) (runs.Step, error) {
			return runs.Step{}, nil
		}}
		started := time.Date(2026, 11, 21, 3, 0, 0, 0, time.UTC)
		errText := "boom"
		tail := "out"

		err := recorder{store: store}.Step(t.Context(), "run-1", flows.StepResult{
			Step: "push", Status: runs.StepFailed, StartedAt: started, FinishedAt: started.Add(time.Second), Error: &errText, OutputTail: &tail,
		})

		require.NoError(t, err)
		call := store.AddRunStepCalls()[0]
		assert.Equal(t, "run-1", call.RunID)
		assert.Equal(t, client.AddRunStepRequest{
			Step: "push", Status: runs.StepFailed, StartedAt: started, FinishedAt: started.Add(time.Second), Error: &errText, OutputTail: &tail,
		}, call.Req)
	})

	t.Run("store error is returned", func(t *testing.T) {
		store := &mockRunStore{AddRunStepFunc: func(context.Context, string, client.AddRunStepRequest) (runs.Step, error) {
			return runs.Step{}, domain.ErrUpstream
		}}

		err := recorder{store: store}.Step(t.Context(), "run-1", flows.StepResult{Step: "push"})

		require.ErrorIs(t, err, domain.ErrUpstream)
	})
}

func TestRecorderFinish(t *testing.T) {
	t.Run("maps status error and summary", func(t *testing.T) {
		store := &mockRunStore{FinishRunFunc: func(context.Context, string, client.FinishRunRequest) (runs.Run, error) {
			return runs.Run{}, nil
		}}
		errText := "failed"
		summary := json.RawMessage(`{"applied":1}`)

		err := recorder{store: store}.Finish(t.Context(), "run-1", runs.StatusFailed, &errText, summary)

		require.NoError(t, err)
		call := store.FinishRunCalls()[0]
		assert.Equal(t, "run-1", call.RunID)
		assert.Equal(t, client.FinishRunRequest{Status: runs.StatusFailed, Error: &errText, Summary: summary}, call.Req)
	})

	t.Run("store error is returned", func(t *testing.T) {
		store := &mockRunStore{FinishRunFunc: func(context.Context, string, client.FinishRunRequest) (runs.Run, error) {
			return runs.Run{}, domain.ErrUpstream
		}}

		err := recorder{store: store}.Finish(t.Context(), "run-1", runs.StatusSucceeded, nil, nil)

		require.ErrorIs(t, err, domain.ErrUpstream)
	})
}
