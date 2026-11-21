package runs

import (
	"context"
	"time"
)

//go:generate go tool moq -out repository_mock.go . Repository:MockRepository
type Repository interface {
	CreateRun(ctx context.Context, r NewRun) (Run, error)
	AddStep(ctx context.Context, s NewStep) (Step, error)
	FinishRun(ctx context.Context, f Finish) (Run, error)
	InterruptRuns(ctx context.Context, flow *string) (int, error)
	ListRuns(ctx context.Context, f ListFilter) ([]Run, error)
	GetRun(ctx context.Context, id string) (Run, error)
	Steps(ctx context.Context, runID string) ([]Step, error)
	PruneRuns(ctx context.Context, olderThan time.Duration) (int, error)
}
