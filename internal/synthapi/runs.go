package synthapi

import (
	"context"
	"net/http"
	"time"

	"claude-memory-sync/internal/pipeline/client"
	"claude-memory-sync/internal/runs"

	"github.com/danielgtaylor/huma/v2"
)

type listRunsInput struct {
	Flow   string    `query:"flow"`
	Status string    `query:"status" enum:"running,succeeded,failed,partial,interrupted"`
	Limit  int       `query:"limit" minimum:"1" maximum:"200" default:"50"`
	Before time.Time `query:"before" format:"date-time"`
}

type listRunsOutput struct {
	Body struct {
		Runs []runs.Run `json:"runs"`
	}
}

type getRunInput struct {
	ID string `path:"id" minLength:"1"`
}

type getRunOutput struct {
	Body runs.RunDetail
}

func (s *server) registerRuns(api huma.API) {
	huma.Register(api, huma.Operation{OperationID: "list-runs", Method: http.MethodGet, Path: "/api/runs"}, s.listRuns)
	huma.Register(api, huma.Operation{OperationID: "get-run", Method: http.MethodGet, Path: "/api/runs/{id}"}, s.getRun)
}

func (s *server) listRuns(ctx context.Context, in *listRunsInput) (*listRunsOutput, error) {
	params := client.ListRunsParams{Flow: in.Flow, Limit: in.Limit}
	if in.Status != "" {
		status := runs.Status(in.Status)
		params.Status = &status
	}
	if !in.Before.IsZero() {
		params.Before = &in.Before
	}

	found, err := s.Pipeline.ListRuns(ctx, params)
	if err != nil {
		return nil, toHumaError(ctx, err)
	}
	if found == nil {
		found = []runs.Run{}
	}

	out := &listRunsOutput{}
	out.Body.Runs = found
	return out, nil
}

func (s *server) getRun(ctx context.Context, in *getRunInput) (*getRunOutput, error) {
	detail, err := s.Pipeline.GetRun(ctx, in.ID)
	if err != nil {
		return nil, toHumaError(ctx, err)
	}
	if detail.Steps == nil {
		detail.Steps = []runs.Step{}
	}
	return &getRunOutput{Body: detail}, nil
}
