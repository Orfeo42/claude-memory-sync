package proposalapply

import (
	"context"
	"errors"

	"claude-memory-sync/internal/domain"
	"claude-memory-sync/internal/ops"
	"claude-memory-sync/internal/pipeline"
	"claude-memory-sync/internal/pipeline/client"
)

type fileGroup struct {
	Name  string
	Files []pipeline.File
}

type Submitter struct {
	pipeline   Pipeline
	sourceFlow string
	sourceRun  string
}

func NewSubmitter(p Pipeline, sourceFlow, sourceRun string) *Submitter {
	return &Submitter{pipeline: p, sourceFlow: sourceFlow, sourceRun: sourceRun}
}

func (s *Submitter) Submit(ctx context.Context, files []pipeline.File) (ops.SubmitOutcome, error) {
	outcome := ops.SubmitOutcome{Created: []string{}, Rejected: []ops.RejectedOp{}}

	for _, group := range groupFiles(files) {
		proposal, err := s.pipeline.Create(ctx, client.CreateRequest{SourceFlow: s.sourceFlow, SourceRun: s.sourceRun, Files: group.Files})
		if err == nil {
			outcome.Created = append(outcome.Created, proposal.ID)
			continue
		}

		var upstream *domain.UpstreamError
		isRejection := errors.Is(err, domain.ErrConflict) || errors.Is(err, domain.ErrValidation)
		if !isRejection || !errors.As(err, &upstream) {
			return ops.SubmitOutcome{}, err
		}
		outcome.Rejected = append(outcome.Rejected, ops.RejectedOp{Name: group.Name, Reason: upstream.Body})
	}

	return outcome, nil
}

func groupFiles(files []pipeline.File) []fileGroup {
	groups := make([]fileGroup, 0, len(files))
	index := make(map[string]int, len(files))
	for _, f := range files {
		name := pipeline.GroupOf(f.TargetPath)
		i, ok := index[name]
		if !ok {
			i = len(groups)
			index[name] = i
			groups = append(groups, fileGroup{Name: name})
		}
		groups[i].Files = append(groups[i].Files, f)
	}
	return groups
}
