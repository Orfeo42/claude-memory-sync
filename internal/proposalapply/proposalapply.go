package proposalapply

import (
	"context"

	"claude-memory-sync/internal/pipeline"
	"claude-memory-sync/internal/pipeline/client"
)

type proposalError string

func (e proposalError) Error() string { return string(e) }

const (
	ErrProposalsFailed       proposalError = "some proposals failed to apply"
	errProposalPathTraversal proposalError = "proposal path is unsafe"
)

//go:generate go tool moq -out pipeline_mock.go . Pipeline:MockPipeline
type Pipeline interface {
	Create(ctx context.Context, req client.CreateRequest) (pipeline.Proposal, error)
	Claim(ctx context.Context, worker string, leaseSeconds int) (pipeline.Proposal, error)
	MarkApplied(ctx context.Context, id, actor, sha string) (pipeline.Proposal, error)
	MarkFailed(ctx context.Context, id, actor, errText string) (pipeline.Proposal, error)
	Expire(ctx context.Context, actor string, olderThanHours int) (int, error)
	AddRejection(ctx context.Context, req client.AddRejectionRequest) error
}
