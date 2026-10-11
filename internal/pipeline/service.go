package pipeline

import (
	"context"
	"log/slog"
	"time"

	"claude-memory-sync/internal/domain"
	"claude-memory-sync/internal/guardrails"
)

const (
	defaultListLimit = 100
	defaultLease     = 10 * time.Minute
	maxLease         = time.Hour
)

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func validateFiles(files []File) (string, error) {
	if len(files) == 0 {
		return "", domain.Error(domain.ErrValidation, "proposal has no files")
	}

	group := GroupOf(files[0].TargetPath)
	seen := make(map[string]bool, len(files))
	for _, f := range files {
		if err := ValidateTarget(f.TargetPath); err != nil {
			return "", domain.WrapError(err, slog.String("target_path", f.TargetPath))
		}
		if seen[f.TargetPath] {
			return "", domain.Error(domain.ErrValidation, "duplicate target path in proposal", slog.String("target_path", f.TargetPath))
		}
		seen[f.TargetPath] = true
		if err := guardrails.ScanSecrets([]byte(f.Content)); err != nil {
			return "", domain.Error(domain.ErrValidation, "proposal content contains a secret", slog.String("target_path", f.TargetPath))
		}
		if GroupOf(f.TargetPath) != group {
			return "", domain.Error(domain.ErrValidation, "proposal files must share one group", slog.String("target_path", f.TargetPath))
		}
	}
	return group, nil
}

func (s *Service) Create(ctx context.Context, sourceFlow, sourceRun string, files []File) (Proposal, error) {
	group, err := validateFiles(files)
	if err != nil {
		return Proposal{}, err
	}

	p, err := s.repo.Create(ctx, NewProposal{
		SourceFlow: sourceFlow,
		SourceRun:  sourceRun,
		Kind:       KindOf(files[0].TargetPath),
		Name:       group,
		Files:      files,
	})
	if err != nil {
		return Proposal{}, domain.WrapError(err, slog.String("source_flow", sourceFlow), slog.String("source_run", sourceRun))
	}

	slog.InfoContext(ctx, "proposal created",
		slog.String("proposal_id", p.ID),
		slog.String("to_status", string(p.Status)),
		slog.String("actor", sourceFlow),
	)
	return p, nil
}

func (s *Service) Get(ctx context.Context, id string) (ProposalDetail, error) {
	p, err := s.repo.Get(ctx, id)
	if err != nil {
		return ProposalDetail{}, domain.WrapError(err, slog.String("proposal_id", id))
	}
	events, err := s.repo.Events(ctx, id)
	if err != nil {
		return ProposalDetail{}, domain.WrapError(err, slog.String("proposal_id", id))
	}
	return ProposalDetail{Proposal: p, Events: events}, nil
}

func (s *Service) List(ctx context.Context, f ListFilter) ([]Proposal, error) {
	if f.Limit == 0 {
		f.Limit = defaultListLimit
	}
	proposals, err := s.repo.List(ctx, f)
	if err != nil {
		return nil, domain.WrapError(err)
	}
	return proposals, nil
}

func (s *Service) transition(ctx context.Context, from, to Status, t Transition) (Proposal, error) {
	if !CanTransition(from, to) {
		return Proposal{}, domain.Error(domain.ErrConflict, "status transition is not allowed",
			slog.String("proposal_id", t.ID),
			slog.String("from_status", string(from)),
			slog.String("to_status", string(to)),
		)
	}

	t.From = from
	t.To = to
	p, err := s.repo.Transition(ctx, t)
	if err != nil {
		return Proposal{}, domain.WrapError(err, slog.String("proposal_id", t.ID))
	}

	slog.InfoContext(ctx, "proposal status changed",
		slog.String("proposal_id", t.ID),
		slog.String("from_status", string(from)),
		slog.String("to_status", string(to)),
		slog.String("actor", t.Actor),
	)
	return p, nil
}

func (s *Service) Decide(ctx context.Context, id, actor string, approve bool, reason *string) (Proposal, error) {
	if approve {
		return s.transition(ctx, StatusPending, StatusApproved, Transition{ID: id, Actor: actor})
	}
	return s.transition(ctx, StatusPending, StatusRejected, Transition{ID: id, Actor: actor, Note: reason})
}

func (s *Service) Revise(ctx context.Context, id, actor string, files []File) (Proposal, error) {
	group, err := validateFiles(files)
	if err != nil {
		return Proposal{}, err
	}

	current, err := s.repo.Get(ctx, id)
	if err != nil {
		return Proposal{}, domain.WrapError(err, slog.String("proposal_id", id))
	}
	if current.Name != group {
		return Proposal{}, domain.Error(domain.ErrValidation, "revised files must keep the proposal group", slog.String("proposal_id", id))
	}

	p, err := s.repo.ReplaceFiles(ctx, id, actor, files)
	if err != nil {
		return Proposal{}, domain.WrapError(err, slog.String("proposal_id", id))
	}

	slog.InfoContext(ctx, "proposal files revised",
		slog.String("proposal_id", id),
		slog.String("actor", actor),
	)
	return p, nil
}

func (s *Service) Retry(ctx context.Context, id, actor string) (Proposal, error) {
	return s.transition(ctx, StatusFailed, StatusApproved, Transition{ID: id, Actor: actor})
}

func (s *Service) MarkApplied(ctx context.Context, id, actor, sha string) (Proposal, error) {
	return s.transition(ctx, StatusApplying, StatusApplied, Transition{ID: id, Actor: actor, AppliedSHA: &sha})
}

func (s *Service) MarkFailed(ctx context.Context, id, actor, errText string) (Proposal, error) {
	return s.transition(ctx, StatusApplying, StatusFailed, Transition{ID: id, Actor: actor, Note: &errText})
}

func (s *Service) Claim(ctx context.Context, worker string, lease time.Duration) (Proposal, error) {
	if lease == 0 {
		lease = defaultLease
	}
	if lease < 0 || lease > maxLease {
		return Proposal{}, domain.Error(domain.ErrValidation, "lease must be between 0 and 1 hour")
	}

	p, err := s.repo.Claim(ctx, worker, lease)
	if err != nil {
		return Proposal{}, domain.WrapError(err, slog.String("actor", worker))
	}

	slog.InfoContext(ctx, "proposal claimed",
		slog.String("proposal_id", p.ID),
		slog.String("from_status", string(StatusApproved)),
		slog.String("to_status", string(StatusApplying)),
		slog.String("actor", worker),
	)
	return p, nil
}

func (s *Service) Expire(ctx context.Context, olderThan time.Duration, actor string) (int, error) {
	if olderThan <= 0 {
		return 0, domain.Error(domain.ErrValidation, "older_than must be positive")
	}

	n, err := s.repo.ExpirePending(ctx, olderThan, actor)
	if err != nil {
		return 0, domain.WrapError(err, slog.String("actor", actor))
	}

	slog.InfoContext(ctx, "pending proposals expired",
		slog.Int("expired", n),
		slog.String("from_status", string(StatusPending)),
		slog.String("to_status", string(StatusExpired)),
		slog.String("actor", actor),
	)
	return n, nil
}

func (s *Service) Rejections(ctx context.Context, kind *Kind) ([]Rejection, error) {
	rejections, err := s.repo.Rejections(ctx, kind)
	if err != nil {
		return nil, domain.WrapError(err)
	}
	return rejections, nil
}

func (s *Service) AddRejection(ctx context.Context, kind Kind, name, actor string, reason *string) error {
	if _, err := ParseKind(string(kind)); err != nil {
		return domain.WrapError(err)
	}
	if name == "" {
		return domain.Error(domain.ErrValidation, "rejection name is required")
	}
	if actor == "" {
		return domain.Error(domain.ErrValidation, "rejection actor is required")
	}

	if err := s.repo.UpsertRejection(ctx, Rejection{Kind: kind, Name: name, Reason: reason}); err != nil {
		return domain.WrapError(err, slog.String("kind", string(kind)), slog.String("name", name), slog.String("actor", actor))
	}

	slog.InfoContext(ctx, "rejection recorded",
		slog.String("kind", string(kind)),
		slog.String("name", name),
		slog.String("actor", actor),
	)
	return nil
}
