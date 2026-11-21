package synthapi

import (
	"context"
	"net/http"
	"slices"
	"strings"

	"claude-memory-sync/internal/domain"
	"claude-memory-sync/internal/flows"
	"claude-memory-sync/internal/llm"
	"claude-memory-sync/internal/ops"
	"claude-memory-sync/internal/pipeline"
	"claude-memory-sync/internal/pipeline/client"
	"claude-memory-sync/internal/runs"
	"claude-memory-sync/internal/synthsettings"

	"github.com/danielgtaylor/huma/v2"
)

const (
	listLimit       = 200
	skillsPrefix    = "global/skills/"
	chatPromptName  = "proposal-chat"
	chatMaxBodySize = 4 * 1024 * 1024
)

type listProposalsInput struct {
	Status string `query:"status" enum:"pending,approved,applying,applied,rejected,expired,failed"`
}

type proposalIDInput struct {
	ID string `path:"id" minLength:"1"`
}

type decisionInput struct {
	ID   string `path:"id" minLength:"1"`
	Body struct {
		Decision string `json:"decision" enum:"approve,reject"`
		Reason   string `json:"reason,omitempty" maxLength:"2000"`
	}
}

type chatTurn struct {
	Role    string `json:"role" enum:"user,assistant"`
	Content string `json:"content" maxLength:"20000"`
}

type chatInput struct {
	ID   string `path:"id" minLength:"1"`
	Body struct {
		Message string     `json:"message" minLength:"1" maxLength:"20000"`
		Model   string     `json:"model" enum:"sonnet,opus,haiku"`
		History []chatTurn `json:"history" maxItems:"50"`
	}
}

type revisionInput struct {
	ID   string `path:"id" minLength:"1"`
	Body struct {
		Reply string `json:"reply" minLength:"1" maxLength:"200000"`
	}
}

type proposalsOutput struct {
	Body struct {
		Proposals []pipeline.Proposal `json:"proposals"`
	}
}

type proposalDetailOutput struct {
	Body pipeline.ProposalDetail
}

type decisionOutput struct {
	Body struct {
		Proposal     pipeline.Proposal `json:"proposal"`
		ApplyStarted bool              `json:"apply_started"`
	}
}

type chatOutput struct {
	Body struct {
		Reply string `json:"reply"`
	}
}

type revisionOutput struct {
	Body struct {
		Applied  []string          `json:"applied"`
		Proposal pipeline.Proposal `json:"proposal"`
	}
}

func (s *server) registerProposals(api huma.API) {
	huma.Register(api, huma.Operation{OperationID: "list-proposals", Method: http.MethodGet, Path: "/api/proposals"}, s.listProposals)
	huma.Register(api, huma.Operation{OperationID: "get-proposal", Method: http.MethodGet, Path: "/api/proposals/{id}"}, s.getProposal)
	huma.Register(api, huma.Operation{OperationID: "decide-proposal", Method: http.MethodPost, Path: "/api/proposals/{id}/decision"}, s.decideProposal)
	huma.Register(api, huma.Operation{OperationID: "retry-proposal", Method: http.MethodPost, Path: "/api/proposals/{id}/retry"}, s.retryProposal)
	huma.Register(api, huma.Operation{OperationID: "chat-proposal", Method: http.MethodPost, Path: "/api/proposals/{id}/chat", MaxBodyBytes: chatMaxBodySize}, s.chatProposal)
	huma.Register(api, huma.Operation{OperationID: "revise-proposal", Method: http.MethodPost, Path: "/api/proposals/{id}/revision"}, s.reviseProposal)
}

func (s *server) listProposals(ctx context.Context, in *listProposalsInput) (*proposalsOutput, error) {
	statuses := []pipeline.Status{pipeline.StatusPending, pipeline.StatusFailed, pipeline.StatusApproved, pipeline.StatusApplying}
	if in.Status != "" {
		statuses = []pipeline.Status{pipeline.Status(in.Status)}
	}

	merged := []pipeline.Proposal{}
	for _, status := range statuses {
		found, err := s.Pipeline.List(ctx, client.ListParams{Status: &status, Limit: listLimit})
		if err != nil {
			return nil, toHumaError(ctx, err)
		}
		merged = append(merged, found...)
	}
	slices.SortFunc(merged, func(a, b pipeline.Proposal) int { return b.CreatedAt.Compare(a.CreatedAt) })

	out := &proposalsOutput{}
	out.Body.Proposals = merged
	return out, nil
}

func (s *server) getProposal(ctx context.Context, in *proposalIDInput) (*proposalDetailOutput, error) {
	detail, err := s.Pipeline.Get(ctx, in.ID)
	if err != nil {
		return nil, toHumaError(ctx, err)
	}
	if detail.Events == nil {
		detail.Events = []pipeline.Event{}
	}
	return &proposalDetailOutput{Body: detail}, nil
}

func (s *server) startApply(ctx context.Context) bool {
	if s.Flows.Running(flows.ProposalApply) {
		return false
	}
	s.launch(ctx, flows.ProposalApply, runs.TriggerEvent)
	return true
}

func (s *server) decideProposal(ctx context.Context, in *decisionInput) (*decisionOutput, error) {
	req := client.DecideRequest{Decision: in.Body.Decision, Actor: synthsettings.Actor, Reason: in.Body.Reason}
	proposal, err := s.Pipeline.Decide(ctx, in.ID, req)
	if err != nil {
		return nil, toHumaError(ctx, err)
	}

	out := &decisionOutput{}
	out.Body.Proposal = proposal
	out.Body.ApplyStarted = in.Body.Decision == "approve" && s.startApply(ctx)
	return out, nil
}

func (s *server) retryProposal(ctx context.Context, in *proposalIDInput) (*decisionOutput, error) {
	proposal, err := s.Pipeline.Retry(ctx, in.ID, synthsettings.Actor)
	if err != nil {
		return nil, toHumaError(ctx, err)
	}

	out := &decisionOutput{}
	out.Body.Proposal = proposal
	out.Body.ApplyStarted = s.startApply(ctx)
	return out, nil
}

func (s *server) chatProposal(ctx context.Context, in *chatInput) (*chatOutput, error) {
	detail, err := s.Pipeline.Get(ctx, in.ID)
	if err != nil {
		return nil, toHumaError(ctx, err)
	}
	if len(detail.Proposal.Files) == 0 {
		return nil, toHumaError(ctx, domain.Error(domain.ErrValidation, "proposal has no files"))
	}
	system, err := s.Settings.Prompt(ctx, chatPromptName)
	if err != nil {
		return nil, toHumaError(ctx, err)
	}

	parts := []string{system, "\n=== staged files ==="}
	for _, file := range detail.Proposal.Files {
		parts = append(parts, "--- "+file.TargetPath+" ---", file.Content)
	}
	parts = append(parts, "\n=== conversation ===")
	for _, turn := range in.Body.History {
		parts = append(parts, "["+turn.Role+"]\n"+turn.Content)
	}
	parts = append(parts, "[user]\n"+in.Body.Message, "\n[assistant]")

	reply, err := s.LLM.Run(ctx, llm.Request{Model: in.Body.Model, Prompt: strings.Join(parts, "\n")})
	if err != nil {
		return nil, toHumaError(ctx, err)
	}

	out := &chatOutput{}
	out.Body.Reply = reply
	return out, nil
}

func (s *server) reviseProposal(ctx context.Context, in *revisionInput) (*revisionOutput, error) {
	parsed, err := ops.Parse(strings.NewReader(in.Body.Reply))
	if err != nil {
		return nil, toHumaError(ctx, domain.Error(domain.ErrValidation, "reply holds malformed file blocks"))
	}
	detail, err := s.Pipeline.Get(ctx, in.ID)
	if err != nil {
		return nil, toHumaError(ctx, err)
	}

	files, applied := mergeRevision(detail.Proposal.Files, parsed.Files)
	if len(applied) == 0 {
		return nil, toHumaError(ctx, domain.Error(domain.ErrValidation, "no applicable file blocks found in the reply"))
	}

	proposal, err := s.Pipeline.Revise(ctx, in.ID, client.ReviseRequest{Actor: synthsettings.Actor, Files: files})
	if err != nil {
		return nil, toHumaError(ctx, err)
	}

	out := &revisionOutput{}
	out.Body.Applied = applied
	out.Body.Proposal = proposal
	return out, nil
}

func mergeRevision(files []pipeline.File, blocks []ops.FileOp) ([]pipeline.File, []string) {
	groups := make([]string, 0, len(files))
	for _, f := range files {
		groups = append(groups, pipeline.GroupOf(f.TargetPath))
	}
	applied := make([]string, 0, len(blocks))
	for _, op := range blocks {
		idx := slices.IndexFunc(files, func(f pipeline.File) bool { return f.TargetPath == op.Name })
		switch {
		case idx >= 0:
			files[idx].Content = string(op.Content)
		case strings.HasPrefix(op.Name, skillsPrefix) && slices.Contains(groups, pipeline.GroupOf(op.Name)):
			files = append(files, pipeline.File{TargetPath: op.Name, Content: string(op.Content)})
		default:
			continue
		}
		applied = append(applied, op.Name)
	}
	return files, applied
}
