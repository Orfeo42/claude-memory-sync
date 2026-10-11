package pipeline

import (
	"context"
	"time"
)

type pipelineError string

func (e pipelineError) Error() string { return string(e) }

const ErrNothingToClaim pipelineError = "no approved proposal to claim"

//go:generate go tool moq -out repository_mock.go . Repository:MockRepository
type Repository interface {
	Create(ctx context.Context, p NewProposal) (Proposal, error)
	Get(ctx context.Context, id string) (Proposal, error)
	List(ctx context.Context, f ListFilter) ([]Proposal, error)
	Events(ctx context.Context, id string) ([]Event, error)
	Transition(ctx context.Context, t Transition) (Proposal, error)
	ReplaceFiles(ctx context.Context, id, actor string, files []File) (Proposal, error)
	Claim(ctx context.Context, worker string, lease time.Duration) (Proposal, error)
	ExpirePending(ctx context.Context, olderThan time.Duration, actor string) (int, error)
	Rejections(ctx context.Context, kind *Kind) ([]Rejection, error)
	UpsertRejection(ctx context.Context, r Rejection) error
}
