package synthapi

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"claude-memory-sync/internal/domain"
	"claude-memory-sync/internal/flows"
	"claude-memory-sync/internal/pipeline/client"
	"claude-memory-sync/internal/runs"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func flowSettings() *MockSettings {
	return &MockSettings{
		ScheduleFunc:      func(_ context.Context, _ flows.Name) (string, error) { return "0 3 * * *", nil },
		SetScheduleFunc:   func(_ context.Context, _ flows.Name, cron string) (string, error) { return cron, nil },
		ResetScheduleFunc: func(_ context.Context, _ flows.Name) (string, error) { return "", nil },
	}
}

func flowPipeline() *MockPipeline {
	return &MockPipeline{ListRunsFunc: func(_ context.Context, p client.ListRunsParams) ([]runs.Run, error) {
		if p.Flow == "taskmine" {
			return []runs.Run{{ID: "r-" + p.Flow, Flow: p.Flow}}, nil
		}
		return nil, nil
	}}
}

func TestListFlows(t *testing.T) {
	t.Run("lists the four flows in order with last run", func(t *testing.T) {
		pl := flowPipeline()
		fl := &MockFlows{RunningFunc: func(f flows.Name) bool { return f == flows.Intake }}
		rec := do(t, New(Deps{Pipeline: pl, Settings: flowSettings(), Flows: fl}), http.MethodGet, "/api/flows", "")

		require.Equal(t, http.StatusOK, rec.Code)
		var body struct {
			Flows []struct {
				Name     string    `json:"name"`
				Schedule string    `json:"schedule"`
				Running  bool      `json:"running"`
				LastRun  *runs.Run `json:"last_run"`
			} `json:"flows"`
		}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
		require.Len(t, body.Flows, 4)
		assert.Equal(t, "intake", body.Flows[0].Name)
		assert.Equal(t, "daily-synth", body.Flows[1].Name)
		assert.Equal(t, "taskmine", body.Flows[2].Name)
		assert.Equal(t, "proposal-apply", body.Flows[3].Name)
		assert.True(t, body.Flows[0].Running)
		assert.False(t, body.Flows[1].Running)
		assert.Nil(t, body.Flows[0].LastRun)
		require.NotNil(t, body.Flows[2].LastRun)
		assert.Equal(t, "r-taskmine", body.Flows[2].LastRun.ID)
		assert.Equal(t, "0 3 * * *", body.Flows[0].Schedule)
		assert.Len(t, pl.ListRunsCalls(), 4)
		assert.Equal(t, 1, pl.ListRunsCalls()[0].Params.Limit)
	})

	t.Run("upstream error is 502", func(t *testing.T) {
		pl := &MockPipeline{ListRunsFunc: func(_ context.Context, _ client.ListRunsParams) ([]runs.Run, error) {
			return nil, domain.Error(domain.ErrUpstream, "down")
		}}
		rec := do(t, New(Deps{Pipeline: pl, Settings: flowSettings(), Flows: &MockFlows{RunningFunc: func(flows.Name) bool { return false }}}), http.MethodGet, "/api/flows", "")
		assert.Equal(t, http.StatusBadGateway, rec.Code)
	})
}

func TestRunFlow(t *testing.T) {
	t.Run("starts the flow and answers 202", func(t *testing.T) {
		started := make(chan struct{}, 1)
		fl := &MockFlows{
			RunningFunc: func(flows.Name) bool { return false },
			RunFunc: func(_ context.Context, _ flows.Name, _ runs.Trigger) (string, error) {
				started <- struct{}{}
				return "r1", nil
			},
		}
		rec := do(t, New(Deps{Flows: fl}), http.MethodPost, "/api/flows/taskmine/run", "")

		require.Equal(t, http.StatusAccepted, rec.Code)
		assert.JSONEq(t, `{"flow":"taskmine"}`, rec.Body.String())
		<-started
		assert.Equal(t, flows.Taskmine, fl.RunCalls()[0].Flow)
		assert.Equal(t, runs.TriggerManual, fl.RunCalls()[0].Trigger)
	})

	t.Run("unknown flow is 404", func(t *testing.T) {
		rec := do(t, New(Deps{Flows: &MockFlows{}}), http.MethodPost, "/api/flows/nope/run", "")
		assert.Equal(t, http.StatusNotFound, rec.Code)
	})

	t.Run("running flow is 409", func(t *testing.T) {
		fl := &MockFlows{RunningFunc: func(flows.Name) bool { return true }}
		rec := do(t, New(Deps{Flows: fl}), http.MethodPost, "/api/flows/intake/run", "")

		assert.Equal(t, http.StatusConflict, rec.Code)
		assert.Empty(t, fl.RunCalls())
	})
}

func TestLaunchLogging(t *testing.T) {
	t.Run("conflict upstream and internal errors all finish", func(t *testing.T) {
		done := make(chan struct{}, 3)
		errs := make(chan error, 3)
		errs <- domain.Error(domain.ErrConflict, "busy")
		errs <- domain.Error(domain.ErrUpstream, "down")
		errs <- assert.AnError
		fl := &MockFlows{RunFunc: func(_ context.Context, _ flows.Name, _ runs.Trigger) (string, error) {
			defer func() { done <- struct{}{} }()
			return "", <-errs
		}}
		s := &server{Deps: Deps{Flows: fl}}
		s.launch(context.Background(), flows.Intake, runs.TriggerManual)
		s.launch(context.Background(), flows.Intake, runs.TriggerManual)
		s.launch(context.Background(), flows.Intake, runs.TriggerManual)
		<-done
		<-done
		<-done
		assert.Len(t, fl.RunCalls(), 3)
	})
}

func TestPutSchedule(t *testing.T) {
	t.Run("sets schedule then reloads scheduler", func(t *testing.T) {
		sc := &MockScheduler{ReloadFunc: func(_ context.Context, _ flows.Name) error { return nil }}
		st := flowSettings()
		deps := Deps{Pipeline: flowPipeline(), Settings: st, Flows: &MockFlows{RunningFunc: func(flows.Name) bool { return false }}, Scheduler: sc}
		rec := do(t, New(deps), http.MethodPut, "/api/flows/intake/schedule", `{"cron":"*/5 * * * *"}`)

		require.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "*/5 * * * *", st.SetScheduleCalls()[0].Cron)
		require.Len(t, sc.ReloadCalls(), 1)
		assert.Equal(t, flows.Intake, sc.ReloadCalls()[0].Flow)
		assert.Contains(t, rec.Body.String(), `"name":"intake"`)
	})

	t.Run("invalid cron from settings is 422 and no reload", func(t *testing.T) {
		sc := &MockScheduler{}
		st := &MockSettings{SetScheduleFunc: func(_ context.Context, _ flows.Name, _ string) (string, error) {
			return "", domain.Error(domain.ErrValidation, "bad cron")
		}}
		rec := do(t, New(Deps{Settings: st, Scheduler: sc}), http.MethodPut, "/api/flows/intake/schedule", `{"cron":"x"}`)

		assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
		assert.Empty(t, sc.ReloadCalls())
	})

	t.Run("unknown flow is 404", func(t *testing.T) {
		rec := do(t, New(Deps{}), http.MethodPut, "/api/flows/nope/schedule", `{"cron":"* * * * *"}`)
		assert.Equal(t, http.StatusNotFound, rec.Code)
	})

	t.Run("reload failure is 500", func(t *testing.T) {
		sc := &MockScheduler{ReloadFunc: func(_ context.Context, _ flows.Name) error { return assert.AnError }}
		rec := do(t, New(Deps{Settings: flowSettings(), Scheduler: sc}), http.MethodPut, "/api/flows/intake/schedule", `{"cron":"* * * * *"}`)
		assert.Equal(t, http.StatusInternalServerError, rec.Code)
	})
}

func TestResetSchedule(t *testing.T) {
	t.Run("resets schedule then reloads scheduler", func(t *testing.T) {
		sc := &MockScheduler{ReloadFunc: func(_ context.Context, _ flows.Name) error { return nil }}
		st := flowSettings()
		deps := Deps{Pipeline: flowPipeline(), Settings: st, Flows: &MockFlows{RunningFunc: func(flows.Name) bool { return false }}, Scheduler: sc}
		rec := do(t, New(deps), http.MethodDelete, "/api/flows/taskmine/schedule", "")

		require.Equal(t, http.StatusOK, rec.Code)
		assert.Len(t, st.ResetScheduleCalls(), 1)
		assert.Equal(t, flows.Taskmine, sc.ReloadCalls()[0].Flow)
	})

	t.Run("unknown flow is 404", func(t *testing.T) {
		rec := do(t, New(Deps{}), http.MethodDelete, "/api/flows/nope/schedule", "")
		assert.Equal(t, http.StatusNotFound, rec.Code)
	})
}
