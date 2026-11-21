package httpapi

import (
	"context"
	"errors"
	"net/http"
	"time"

	"claude-memory-sync/internal/pipeline"

	"github.com/danielgtaylor/huma/v2"
)

const healthzPath = "/v1/healthz"

type controller struct {
	svc *pipeline.Service
}

type emptyInput struct{}

type emptyOutput struct{}

type proposalOutput struct {
	Body pipeline.Proposal
}

type createInput struct {
	Body struct {
		SourceFlow string          `json:"source_flow" minLength:"1"`
		SourceRun  string          `json:"source_run" minLength:"1"`
		Files      []pipeline.File `json:"files" minItems:"1"`
	}
}

type listInput struct {
	Status string    `query:"status" enum:"pending,approved,applying,applied,rejected,expired,failed"`
	Kind   string    `query:"kind" enum:"skill,rule,agent,global,project-entry,bug,one-time-task,code-change"`
	Before time.Time `query:"before" format:"date-time"`
	Limit  int       `query:"limit" minimum:"1" maximum:"500" default:"100"`
}

type listOutput struct {
	Body struct {
		Proposals []pipeline.Proposal `json:"proposals"`
	}
}

type getInput struct {
	ID string `path:"id" minLength:"1"`
}

type getOutput struct {
	Body pipeline.ProposalDetail
}

type decisionInput struct {
	ID   string `path:"id" minLength:"1"`
	Body struct {
		Decision string  `json:"decision" enum:"approve,reject"`
		Actor    string  `json:"actor" minLength:"1"`
		Reason   *string `json:"reason,omitempty"`
	}
}

type reviseInput struct {
	ID   string `path:"id" minLength:"1"`
	Body struct {
		Actor string          `json:"actor" minLength:"1"`
		Files []pipeline.File `json:"files" minItems:"1"`
	}
}

type actorInput struct {
	ID   string `path:"id" minLength:"1"`
	Body struct {
		Actor string `json:"actor" minLength:"1"`
	}
}

type claimInput struct {
	Body struct {
		Worker       string `json:"worker" minLength:"1"`
		LeaseSeconds int    `json:"lease_seconds,omitempty" minimum:"1" maximum:"3600"`
	}
}

type claimOutput struct {
	Status int
	Body   *pipeline.Proposal
}

type appliedInput struct {
	ID   string `path:"id" minLength:"1"`
	Body struct {
		Actor string `json:"actor" minLength:"1"`
		SHA   string `json:"sha" minLength:"1"`
	}
}

type failedInput struct {
	ID   string `path:"id" minLength:"1"`
	Body struct {
		Actor string `json:"actor" minLength:"1"`
		Error string `json:"error" minLength:"1"`
	}
}

type expireInput struct {
	Body struct {
		Actor          string `json:"actor" minLength:"1"`
		OlderThanHours int    `json:"older_than_hours" minimum:"1"`
	}
}

type expireOutput struct {
	Body struct {
		Expired int `json:"expired"`
	}
}

type rejectionsInput struct {
	Kind string `query:"kind" enum:"skill,rule,agent,global,project-entry,bug,one-time-task,code-change"`
}

type rejectionsOutput struct {
	Body struct {
		Rejections []pipeline.Rejection `json:"rejections"`
	}
}

type addRejectionInput struct {
	Body struct {
		Kind   string  `json:"kind" enum:"skill,rule,agent,global,project-entry,bug,one-time-task,code-change"`
		Name   string  `json:"name" minLength:"1"`
		Actor  string  `json:"actor" minLength:"1"`
		Reason *string `json:"reason,omitempty"`
	}
}

func Operation(id, method, path, role string, status int) huma.Operation {
	return huma.Operation{
		OperationID:   id,
		Method:        method,
		Path:          path,
		DefaultStatus: status,
		Metadata:      map[string]any{roleKey: role},
	}
}

func registerOperations(api huma.API, c *controller) {
	huma.Register(api, Operation("healthz", http.MethodGet, healthzPath, RoleNone, http.StatusOK), c.healthz)
	huma.Register(api, Operation("create-proposal", http.MethodPost, "/v1/proposals", RoleWriter, http.StatusCreated), c.create)
	huma.Register(api, Operation("list-proposals", http.MethodGet, "/v1/proposals", RoleWriter, http.StatusOK), c.list)
	huma.Register(api, Operation("claim-proposal", http.MethodPost, "/v1/proposals/claim", RoleWriter, http.StatusOK), c.claim)
	huma.Register(api, Operation("expire-proposals", http.MethodPost, "/v1/proposals/expire", RoleWriter, http.StatusOK), c.expire)
	huma.Register(api, Operation("get-proposal", http.MethodGet, "/v1/proposals/{id}", RoleWriter, http.StatusOK), c.get)
	huma.Register(api, Operation("decide-proposal", http.MethodPost, "/v1/proposals/{id}/decision", RoleAdmin, http.StatusOK), c.decide)
	huma.Register(api, Operation("revise-proposal", http.MethodPut, "/v1/proposals/{id}/files", RoleAdmin, http.StatusOK), c.revise)
	huma.Register(api, Operation("retry-proposal", http.MethodPost, "/v1/proposals/{id}/retry", RoleAdmin, http.StatusOK), c.retry)
	huma.Register(api, Operation("applied-proposal", http.MethodPost, "/v1/proposals/{id}/applied", RoleWriter, http.StatusOK), c.applied)
	huma.Register(api, Operation("failed-proposal", http.MethodPost, "/v1/proposals/{id}/failed", RoleWriter, http.StatusOK), c.failed)
	huma.Register(api, Operation("list-rejections", http.MethodGet, "/v1/rejections", RoleWriter, http.StatusOK), c.rejections)
	huma.Register(api, Operation("add-rejection", http.MethodPost, "/v1/rejections", RoleAdmin, http.StatusNoContent), c.addRejection)
}

func (c *controller) healthz(_ context.Context, _ *emptyInput) (*emptyOutput, error) {
	return &emptyOutput{}, nil
}

func (c *controller) create(ctx context.Context, in *createInput) (*proposalOutput, error) {
	p, err := c.svc.Create(ctx, in.Body.SourceFlow, in.Body.SourceRun, in.Body.Files)
	if err != nil {
		return nil, ToHumaError(ctx, err)
	}
	return &proposalOutput{Body: p}, nil
}

func (c *controller) list(ctx context.Context, in *listInput) (*listOutput, error) {
	filter := pipeline.ListFilter{Limit: in.Limit}

	if in.Status != "" {
		status, err := pipeline.ParseStatus(in.Status)
		if err != nil {
			return nil, ToHumaError(ctx, err)
		}
		filter.Status = &status
	}
	if in.Kind != "" {
		kind, err := pipeline.ParseKind(in.Kind)
		if err != nil {
			return nil, ToHumaError(ctx, err)
		}
		filter.Kind = &kind
	}
	if !in.Before.IsZero() {
		filter.Before = &in.Before
	}

	proposals, err := c.svc.List(ctx, filter)
	if err != nil {
		return nil, ToHumaError(ctx, err)
	}

	out := &listOutput{}
	out.Body.Proposals = proposals
	if out.Body.Proposals == nil {
		out.Body.Proposals = []pipeline.Proposal{}
	}
	return out, nil
}

func (c *controller) get(ctx context.Context, in *getInput) (*getOutput, error) {
	detail, err := c.svc.Get(ctx, in.ID)
	if err != nil {
		return nil, ToHumaError(ctx, err)
	}
	if detail.Events == nil {
		detail.Events = []pipeline.Event{}
	}
	return &getOutput{Body: detail}, nil
}

func (c *controller) decide(ctx context.Context, in *decisionInput) (*proposalOutput, error) {
	p, err := c.svc.Decide(ctx, in.ID, in.Body.Actor, in.Body.Decision == "approve", in.Body.Reason)
	if err != nil {
		return nil, ToHumaError(ctx, err)
	}
	return &proposalOutput{Body: p}, nil
}

func (c *controller) revise(ctx context.Context, in *reviseInput) (*proposalOutput, error) {
	p, err := c.svc.Revise(ctx, in.ID, in.Body.Actor, in.Body.Files)
	if err != nil {
		return nil, ToHumaError(ctx, err)
	}
	return &proposalOutput{Body: p}, nil
}

func (c *controller) retry(ctx context.Context, in *actorInput) (*proposalOutput, error) {
	p, err := c.svc.Retry(ctx, in.ID, in.Body.Actor)
	if err != nil {
		return nil, ToHumaError(ctx, err)
	}
	return &proposalOutput{Body: p}, nil
}

func (c *controller) claim(ctx context.Context, in *claimInput) (*claimOutput, error) {
	lease := time.Duration(in.Body.LeaseSeconds) * time.Second
	p, err := c.svc.Claim(ctx, in.Body.Worker, lease)
	if errors.Is(err, pipeline.ErrNothingToClaim) {
		return &claimOutput{Status: http.StatusNoContent}, nil
	}
	if err != nil {
		return nil, ToHumaError(ctx, err)
	}
	return &claimOutput{Status: http.StatusOK, Body: &p}, nil
}

func (c *controller) applied(ctx context.Context, in *appliedInput) (*proposalOutput, error) {
	p, err := c.svc.MarkApplied(ctx, in.ID, in.Body.Actor, in.Body.SHA)
	if err != nil {
		return nil, ToHumaError(ctx, err)
	}
	return &proposalOutput{Body: p}, nil
}

func (c *controller) failed(ctx context.Context, in *failedInput) (*proposalOutput, error) {
	p, err := c.svc.MarkFailed(ctx, in.ID, in.Body.Actor, in.Body.Error)
	if err != nil {
		return nil, ToHumaError(ctx, err)
	}
	return &proposalOutput{Body: p}, nil
}

func (c *controller) expire(ctx context.Context, in *expireInput) (*expireOutput, error) {
	n, err := c.svc.Expire(ctx, time.Duration(in.Body.OlderThanHours)*time.Hour, in.Body.Actor)
	if err != nil {
		return nil, ToHumaError(ctx, err)
	}
	out := &expireOutput{}
	out.Body.Expired = n
	return out, nil
}

func (c *controller) rejections(ctx context.Context, in *rejectionsInput) (*rejectionsOutput, error) {
	var kind *pipeline.Kind
	if in.Kind != "" {
		parsed, err := pipeline.ParseKind(in.Kind)
		if err != nil {
			return nil, ToHumaError(ctx, err)
		}
		kind = &parsed
	}

	rejections, err := c.svc.Rejections(ctx, kind)
	if err != nil {
		return nil, ToHumaError(ctx, err)
	}

	out := &rejectionsOutput{}
	out.Body.Rejections = rejections
	if out.Body.Rejections == nil {
		out.Body.Rejections = []pipeline.Rejection{}
	}
	return out, nil
}

func (c *controller) addRejection(ctx context.Context, in *addRejectionInput) (*emptyOutput, error) {
	err := c.svc.AddRejection(ctx, pipeline.Kind(in.Body.Kind), in.Body.Name, in.Body.Actor, in.Body.Reason)
	if err != nil {
		return nil, ToHumaError(ctx, err)
	}
	return &emptyOutput{}, nil
}
