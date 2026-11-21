package synthapi

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"claude-memory-sync/internal/domain"
	"claude-memory-sync/internal/flows"
	"claude-memory-sync/internal/llm"
	"claude-memory-sync/internal/pipeline"
	"claude-memory-sync/internal/pipeline/client"
	"claude-memory-sync/internal/runs"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var t0 = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func TestListProposals(t *testing.T) {
	t.Run("default merges four statuses newest first", func(t *testing.T) {
		var statuses []pipeline.Status
		pl := &MockPipeline{ListFunc: func(_ context.Context, p client.ListParams) ([]pipeline.Proposal, error) {
			statuses = append(statuses, *p.Status)
			assert.Equal(t, 200, p.Limit)
			byStatus := map[pipeline.Status][]pipeline.Proposal{
				pipeline.StatusPending:  {testProposal("p1", pipeline.StatusPending, t0)},
				pipeline.StatusFailed:   {testProposal("f1", pipeline.StatusFailed, t0.Add(2*time.Hour))},
				pipeline.StatusApproved: {testProposal("a1", pipeline.StatusApproved, t0.Add(time.Hour))},
			}
			return byStatus[*p.Status], nil
		}}
		rec := do(t, New(Deps{Pipeline: pl}), http.MethodGet, "/api/proposals", "")

		require.Equal(t, http.StatusOK, rec.Code)
		var body struct {
			Proposals []pipeline.Proposal `json:"proposals"`
		}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
		require.Len(t, body.Proposals, 3)
		assert.Equal(t, "f1", body.Proposals[0].ID)
		assert.Equal(t, "a1", body.Proposals[1].ID)
		assert.Equal(t, "p1", body.Proposals[2].ID)
		assert.Equal(t, []pipeline.Status{pipeline.StatusPending, pipeline.StatusFailed, pipeline.StatusApproved, pipeline.StatusApplying}, statuses)
	})

	t.Run("status filter makes a single call", func(t *testing.T) {
		pl := &MockPipeline{ListFunc: func(_ context.Context, _ client.ListParams) ([]pipeline.Proposal, error) {
			return nil, nil
		}}
		rec := do(t, New(Deps{Pipeline: pl}), http.MethodGet, "/api/proposals?status=applied", "")

		require.Equal(t, http.StatusOK, rec.Code)
		assert.JSONEq(t, `{"proposals":[]}`, rec.Body.String())
		require.Len(t, pl.ListCalls(), 1)
		assert.Equal(t, pipeline.StatusApplied, *pl.ListCalls()[0].Params.Status)
	})

	t.Run("invalid status is 422", func(t *testing.T) {
		rec := do(t, New(Deps{Pipeline: &MockPipeline{}}), http.MethodGet, "/api/proposals?status=bogus", "")
		assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	})

	t.Run("upstream error is 502", func(t *testing.T) {
		pl := &MockPipeline{ListFunc: func(_ context.Context, _ client.ListParams) ([]pipeline.Proposal, error) {
			return nil, domain.Error(domain.ErrUpstream, "pipeline down")
		}}
		rec := do(t, New(Deps{Pipeline: pl}), http.MethodGet, "/api/proposals", "")
		assert.Equal(t, http.StatusBadGateway, rec.Code)
	})
}

func TestGetProposal(t *testing.T) {
	t.Run("returns detail with empty events list", func(t *testing.T) {
		pl := &MockPipeline{GetFunc: func(_ context.Context, id string) (pipeline.ProposalDetail, error) {
			return pipeline.ProposalDetail{Proposal: testProposal(id, pipeline.StatusPending, t0)}, nil
		}}
		rec := do(t, New(Deps{Pipeline: pl}), http.MethodGet, "/api/proposals/p1", "")

		require.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), `"events":[]`)
		assert.Contains(t, rec.Body.String(), `"id":"p1"`)
	})

	t.Run("not found is 404", func(t *testing.T) {
		pl := &MockPipeline{GetFunc: func(_ context.Context, _ string) (pipeline.ProposalDetail, error) {
			return pipeline.ProposalDetail{}, domain.Error(domain.ErrNotFound, "no such proposal")
		}}
		rec := do(t, New(Deps{Pipeline: pl}), http.MethodGet, "/api/proposals/x", "")
		assert.Equal(t, http.StatusNotFound, rec.Code)
	})
}

func decisionFixtures(started chan struct{}, running bool) (*MockPipeline, *MockFlows) {
	pl := &MockPipeline{
		DecideFunc: func(_ context.Context, id string, _ client.DecideRequest) (pipeline.Proposal, error) {
			return testProposal(id, pipeline.StatusApproved, t0), nil
		},
		RetryFunc: func(_ context.Context, id, _ string) (pipeline.Proposal, error) {
			return testProposal(id, pipeline.StatusPending, t0), nil
		},
	}
	fl := &MockFlows{
		RunningFunc: func(_ flows.Name) bool { return running },
		RunFunc: func(_ context.Context, _ flows.Name, _ runs.Trigger) (string, error) {
			started <- struct{}{}
			return "run1", nil
		},
	}
	return pl, fl
}

func TestDecideProposal(t *testing.T) {
	t.Run("approve starts apply when idle", func(t *testing.T) {
		started := make(chan struct{}, 1)
		pl, fl := decisionFixtures(started, false)
		rec := do(t, New(Deps{Pipeline: pl, Flows: fl}), http.MethodPost, "/api/proposals/p1/decision", `{"decision":"approve"}`)

		require.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), `"apply_started":true`)
		<-started
		call := fl.RunCalls()[0]
		assert.Equal(t, flows.ProposalApply, call.Flow)
		assert.Equal(t, runs.TriggerEvent, call.Trigger)
		assert.Equal(t, "ui", pl.DecideCalls()[0].Req.Actor)
		assert.Equal(t, "approve", pl.DecideCalls()[0].Req.Decision)
	})

	t.Run("approve does not start apply when running", func(t *testing.T) {
		started := make(chan struct{}, 1)
		pl, fl := decisionFixtures(started, true)
		rec := do(t, New(Deps{Pipeline: pl, Flows: fl}), http.MethodPost, "/api/proposals/p1/decision", `{"decision":"approve"}`)

		require.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), `"apply_started":false`)
		assert.Empty(t, fl.RunCalls())
	})

	t.Run("reject passes reason and never starts apply", func(t *testing.T) {
		started := make(chan struct{}, 1)
		pl, fl := decisionFixtures(started, false)
		rec := do(t, New(Deps{Pipeline: pl, Flows: fl}), http.MethodPost, "/api/proposals/p1/decision", `{"decision":"reject","reason":"nope"}`)

		require.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), `"apply_started":false`)
		assert.Equal(t, "nope", pl.DecideCalls()[0].Req.Reason)
		assert.Empty(t, fl.RunCalls())
	})

	t.Run("invalid decision is 422", func(t *testing.T) {
		started := make(chan struct{}, 1)
		pl, fl := decisionFixtures(started, false)
		rec := do(t, New(Deps{Pipeline: pl, Flows: fl}), http.MethodPost, "/api/proposals/p1/decision", `{"decision":"maybe"}`)
		assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	})

	t.Run("conflict from pipeline is 409", func(t *testing.T) {
		pl := &MockPipeline{DecideFunc: func(_ context.Context, _ string, _ client.DecideRequest) (pipeline.Proposal, error) {
			return pipeline.Proposal{}, domain.Error(domain.ErrConflict, "already decided")
		}}
		rec := do(t, New(Deps{Pipeline: pl, Flows: &MockFlows{}}), http.MethodPost, "/api/proposals/p1/decision", `{"decision":"approve"}`)
		assert.Equal(t, http.StatusConflict, rec.Code)
	})
}

func TestRetryProposal(t *testing.T) {
	t.Run("starts apply when idle", func(t *testing.T) {
		started := make(chan struct{}, 1)
		pl, fl := decisionFixtures(started, false)
		rec := do(t, New(Deps{Pipeline: pl, Flows: fl}), http.MethodPost, "/api/proposals/p1/retry", "")

		require.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), `"apply_started":true`)
		<-started
		assert.Equal(t, "ui", pl.RetryCalls()[0].Actor)
	})

	t.Run("does not start apply when running", func(t *testing.T) {
		started := make(chan struct{}, 1)
		pl, fl := decisionFixtures(started, true)
		rec := do(t, New(Deps{Pipeline: pl, Flows: fl}), http.MethodPost, "/api/proposals/p1/retry", "")

		require.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), `"apply_started":false`)
		assert.Empty(t, fl.RunCalls())
	})

	t.Run("upstream error is 502", func(t *testing.T) {
		pl := &MockPipeline{RetryFunc: func(_ context.Context, _, _ string) (pipeline.Proposal, error) {
			return pipeline.Proposal{}, domain.Error(domain.ErrUpstream, "down")
		}}
		rec := do(t, New(Deps{Pipeline: pl, Flows: &MockFlows{}}), http.MethodPost, "/api/proposals/p1/retry", "")
		assert.Equal(t, http.StatusBadGateway, rec.Code)
	})
}

func chatFixtures(reply string) (*MockPipeline, *MockSettings, *MockLLM) {
	pl := &MockPipeline{GetFunc: func(_ context.Context, id string) (pipeline.ProposalDetail, error) {
		p := testProposal(id, pipeline.StatusPending, t0)
		p.Files = append(p.Files, pipeline.File{TargetPath: "global/skills/a/ref.md", Content: "ref\n"})
		return pipeline.ProposalDetail{Proposal: p}, nil
	}}
	st := &MockSettings{PromptFunc: func(_ context.Context, _ string) (string, error) { return "SYSTEM", nil }}
	lm := &MockLLM{RunFunc: func(_ context.Context, _ llm.Request) (string, error) { return reply, nil }}
	return pl, st, lm
}

func TestChatProposal(t *testing.T) {
	t.Run("assembles the prompt exactly", func(t *testing.T) {
		pl, st, lm := chatFixtures("hello")
		body := `{"message":"now?","model":"opus","history":[{"role":"user","content":"hi"},{"role":"assistant","content":"yo"}]}`
		rec := do(t, New(Deps{Pipeline: pl, Settings: st, LLM: lm}), http.MethodPost, "/api/proposals/p1/chat", body)

		require.Equal(t, http.StatusOK, rec.Code)
		assert.JSONEq(t, `{"reply":"hello"}`, rec.Body.String())
		want := "SYSTEM\n\n=== staged files ===\n--- global/skills/a/SKILL.md ---\nold\n\n--- global/skills/a/ref.md ---\nref\n\n\n=== conversation ===\n[user]\nhi\n[assistant]\nyo\n[user]\nnow?\n\n[assistant]"
		require.Len(t, lm.RunCalls(), 1)
		assert.Equal(t, want, lm.RunCalls()[0].Req.Prompt)
		assert.Equal(t, "opus", lm.RunCalls()[0].Req.Model)
		assert.Equal(t, "proposal-chat", st.PromptCalls()[0].Name)
	})

	t.Run("invalid model is 422", func(t *testing.T) {
		pl, st, lm := chatFixtures("x")
		rec := do(t, New(Deps{Pipeline: pl, Settings: st, LLM: lm}), http.MethodPost, "/api/proposals/p1/chat", `{"message":"m","model":"gpt","history":[]}`)
		assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	})

	t.Run("empty message is 422", func(t *testing.T) {
		pl, st, lm := chatFixtures("x")
		rec := do(t, New(Deps{Pipeline: pl, Settings: st, LLM: lm}), http.MethodPost, "/api/proposals/p1/chat", `{"message":"","model":"haiku","history":[]}`)
		assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	})

	t.Run("proposal without files is 422", func(t *testing.T) {
		pl := &MockPipeline{GetFunc: func(_ context.Context, id string) (pipeline.ProposalDetail, error) {
			return pipeline.ProposalDetail{Proposal: pipeline.Proposal{ID: id}}, nil
		}}
		rec := do(t, New(Deps{Pipeline: pl}), http.MethodPost, "/api/proposals/p1/chat", `{"message":"m","model":"haiku","history":[]}`)
		assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	})

	t.Run("llm upstream error is 502", func(t *testing.T) {
		pl, st, _ := chatFixtures("x")
		lm := &MockLLM{RunFunc: func(_ context.Context, _ llm.Request) (string, error) {
			return "", domain.Error(domain.ErrUpstream, "claude failed")
		}}
		rec := do(t, New(Deps{Pipeline: pl, Settings: st, LLM: lm}), http.MethodPost, "/api/proposals/p1/chat", `{"message":"m","model":"haiku","history":[]}`)
		assert.Equal(t, http.StatusBadGateway, rec.Code)
	})
}

func TestReviseProposal(t *testing.T) {
	revisePipeline := func() *MockPipeline {
		pl, _, _ := chatFixtures("")
		pl.ReviseFunc = func(_ context.Context, id string, _ client.ReviseRequest) (pipeline.Proposal, error) {
			return testProposal(id, pipeline.StatusPending, t0), nil
		}
		return pl
	}

	t.Run("applies known targets and ignores unknown", func(t *testing.T) {
		pl := revisePipeline()
		reply := "text\n===FILE: global/skills/a/SKILL.md===\nnew body\n===END===\n===FILE: global/skills/zzz/x.md===\nother\n===END===\n"
		payload, err := json.Marshal(map[string]string{"reply": reply})
		require.NoError(t, err)
		rec := do(t, New(Deps{Pipeline: pl}), http.MethodPost, "/api/proposals/p1/revision", string(payload))

		require.Equal(t, http.StatusOK, rec.Code)
		var body struct {
			Applied []string `json:"applied"`
		}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
		assert.Equal(t, []string{"global/skills/a/SKILL.md"}, body.Applied)
		call := pl.ReviseCalls()[0]
		assert.Equal(t, "ui", call.Req.Actor)
		assert.Equal(t, []pipeline.File{
			{TargetPath: "global/skills/a/SKILL.md", Content: "new body\n"},
			{TargetPath: "global/skills/a/ref.md", Content: "ref\n"},
		}, call.Req.Files)
	})

	t.Run("new file in same skill group is applied and appended", func(t *testing.T) {
		pl := revisePipeline()
		payload, err := json.Marshal(map[string]string{"reply": "===FILE: global/skills/a/extra.md===\nx\n===END===\n"})
		require.NoError(t, err)
		rec := do(t, New(Deps{Pipeline: pl}), http.MethodPost, "/api/proposals/p1/revision", string(payload))

		require.Equal(t, http.StatusOK, rec.Code)
		files := pl.ReviseCalls()[0].Req.Files
		require.Len(t, files, 3)
		assert.Equal(t, pipeline.File{TargetPath: "global/skills/a/extra.md", Content: "x\n"}, files[2])
	})

	t.Run("new file in another skill group is ignored", func(t *testing.T) {
		pl := revisePipeline()
		payload, err := json.Marshal(map[string]string{"reply": "===FILE: global/skills/b/SKILL.md===\nx\n===END===\n"})
		require.NoError(t, err)
		rec := do(t, New(Deps{Pipeline: pl}), http.MethodPost, "/api/proposals/p1/revision", string(payload))

		assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
		assert.Empty(t, pl.ReviseCalls())
	})

	t.Run("new non-skill file is ignored", func(t *testing.T) {
		pl := revisePipeline()
		payload, err := json.Marshal(map[string]string{"reply": "===FILE: global/rules/a.md===\nx\n===END===\n"})
		require.NoError(t, err)
		rec := do(t, New(Deps{Pipeline: pl}), http.MethodPost, "/api/proposals/p1/revision", string(payload))

		assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
		assert.Empty(t, pl.ReviseCalls())
	})

	t.Run("no applicable block is 422", func(t *testing.T) {
		pl := revisePipeline()
		payload, err := json.Marshal(map[string]string{"reply": "===FILE: unknown.md===\nx\n===END===\n"})
		require.NoError(t, err)
		rec := do(t, New(Deps{Pipeline: pl}), http.MethodPost, "/api/proposals/p1/revision", string(payload))

		assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
		assert.Empty(t, pl.ReviseCalls())
	})

	t.Run("reply without blocks is 422", func(t *testing.T) {
		pl := revisePipeline()
		rec := do(t, New(Deps{Pipeline: pl}), http.MethodPost, "/api/proposals/p1/revision", `{"reply":"just words"}`)
		assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	})

	t.Run("unterminated block is 422", func(t *testing.T) {
		pl := revisePipeline()
		rec := do(t, New(Deps{Pipeline: pl}), http.MethodPost, "/api/proposals/p1/revision", `{"reply":"===FILE: global/skills/a/SKILL.md===\nx"}`)
		assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	})

	t.Run("upstream error on revise is 502", func(t *testing.T) {
		pl := revisePipeline()
		pl.ReviseFunc = func(_ context.Context, _ string, _ client.ReviseRequest) (pipeline.Proposal, error) {
			return pipeline.Proposal{}, domain.Error(domain.ErrUpstream, "down")
		}
		payload, err := json.Marshal(map[string]string{"reply": "===FILE: global/skills/a/SKILL.md===\nn\n===END===\n"})
		require.NoError(t, err)
		rec := do(t, New(Deps{Pipeline: pl}), http.MethodPost, "/api/proposals/p1/revision", string(payload))
		assert.Equal(t, http.StatusBadGateway, rec.Code)
	})
}
