package synthapi

import (
	"context"
	"net/http"
	"time"

	"claude-memory-sync/internal/footprint"

	"github.com/danielgtaylor/huma/v2"
)

type footprintOutput struct {
	Body footprint.Report
}

func (s *server) registerReport(api huma.API) {
	huma.Register(api, huma.Operation{OperationID: "get-footprint", Method: http.MethodGet, Path: "/api/report/footprint"}, s.getFootprint)
}

func (s *server) getFootprint(ctx context.Context, _ *emptyInput) (*footprintOutput, error) {
	report, err := s.Footprint(s.WorkDir, time.Now())
	if err != nil {
		return nil, toHumaError(ctx, err)
	}
	if report.Projects == nil {
		report.Projects = []footprint.Project{}
	}
	if report.Global.RulesUnconditional == nil {
		report.Global.RulesUnconditional = []string{}
	}
	if report.Global.RulesScoped == nil {
		report.Global.RulesScoped = []string{}
	}
	return &footprintOutput{Body: report}, nil
}
