package proposalapply

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"

	"claude-memory-sync/internal/domain"
	"claude-memory-sync/internal/gitutil"
	"claude-memory-sync/internal/guardrails"
	"claude-memory-sync/internal/pipeline"
	"claude-memory-sync/internal/workrepo"
)

type ApplyApprovedOptions struct {
	WorkDir string
	Lock    string
	Worker  string
}

type AppliedProposal struct {
	ID      string        `json:"id"`
	Kind    pipeline.Kind `json:"kind"`
	Name    string        `json:"name"`
	Targets []string      `json:"targets"`
}

type ApplyApprovedResult struct {
	Applied          []string          `json:"applied"`
	Failed           []string          `json:"failed"`
	AppliedProposals []AppliedProposal `json:"applied_proposals"`
}

type committedProposal struct {
	ID      string
	SHA     string
	Kind    pipeline.Kind
	Name    string
	Targets []string
}

func ApplyApproved(ctx context.Context, p Pipeline, opts ApplyApprovedOptions) (ApplyApprovedResult, error) {
	result := ApplyApprovedResult{Applied: []string{}, Failed: []string{}, AppliedProposals: []AppliedProposal{}}

	committed, err := claimAndCommit(ctx, p, opts, &result)
	if err != nil {
		return result, err
	}

	if err := finalizeCommitted(ctx, p, opts, committed, &result); err != nil {
		return result, err
	}

	if len(result.Failed) > 0 {
		return result, domain.Error(ErrProposalsFailed, "apply-approved finished with failures", slog.Int("failed", len(result.Failed)))
	}
	return result, nil
}

func claimAndCommit(ctx context.Context, p Pipeline, opts ApplyApprovedOptions, result *ApplyApprovedResult) ([]committedProposal, error) {
	committed := make([]committedProposal, 0)

	for {
		proposal, err := p.Claim(ctx, opts.Worker, 0)
		if errors.Is(err, pipeline.ErrNothingToClaim) {
			return committed, nil
		}
		if err != nil {
			return committed, err
		}

		sha, applyErr := applyClaimed(ctx, opts.WorkDir, opts.Lock, proposal)
		if applyErr == nil {
			targets := make([]string, 0, len(proposal.Files))
			for _, f := range proposal.Files {
				targets = append(targets, f.TargetPath)
			}
			committed = append(committed, committedProposal{ID: proposal.ID, SHA: sha, Kind: proposal.Kind, Name: proposal.Name, Targets: targets})
			continue
		}

		if _, err := p.MarkFailed(ctx, proposal.ID, opts.Worker, applyErr.Error()); err != nil {
			return committed, err
		}
		result.Failed = append(result.Failed, proposal.ID)
	}
}

func finalizeCommitted(ctx context.Context, p Pipeline, opts ApplyApprovedOptions, committed []committedProposal, result *ApplyApprovedResult) error {
	if len(committed) == 0 {
		return nil
	}

	if pushErr := workrepo.Push(ctx, opts.WorkDir, opts.Lock); pushErr != nil {
		for _, c := range committed {
			if _, err := p.MarkFailed(ctx, c.ID, opts.Worker, "push failed: "+pushErr.Error()); err != nil {
				return err
			}
			result.Failed = append(result.Failed, c.ID)
		}
		return pushErr
	}

	for _, c := range committed {
		if _, err := p.MarkApplied(ctx, c.ID, opts.Worker, c.SHA); err != nil {
			return err
		}
		result.Applied = append(result.Applied, c.ID)
		result.AppliedProposals = append(result.AppliedProposals, AppliedProposal{ID: c.ID, Kind: c.Kind, Name: c.Name, Targets: c.Targets})
	}
	return nil
}

func resolveProposalDests(workDir string, proposal pipeline.Proposal) ([]string, error) {
	if len(proposal.Files) == 0 {
		return nil, domain.Error(domain.ErrValidation, "proposal has no files")
	}

	dests := make([]string, 0, len(proposal.Files))
	for _, f := range proposal.Files {
		if err := pipeline.ValidateTarget(f.TargetPath); err != nil {
			return nil, domain.Error(err, "reject proposal target")
		}
		if err := guardrails.ScanSecrets([]byte(f.Content)); err != nil {
			return nil, domain.Error(err, "proposal secret scan failed")
		}
		dest, err := resolveProposalDest(workDir, f.TargetPath)
		if err != nil {
			return nil, err
		}
		dests = append(dests, dest)
	}
	return dests, nil
}

func applyClaimed(ctx context.Context, workDir, lock string, proposal pipeline.Proposal) (string, error) {
	dests, err := resolveProposalDests(workDir, proposal)
	if err != nil {
		return "", err
	}

	message := "proposal: apply " + string(proposal.Kind) + " " + proposal.Name
	pathspec := pipeline.PathspecOf(proposal.Files[0].TargetPath)
	sha := ""

	err = gitutil.WithFileLock(lock, func() error {
		for i, f := range proposal.Files {
			if err := os.MkdirAll(filepath.Dir(dests[i]), 0o750); err != nil {
				return domain.Error(err, "create proposal target directory failed")
			}
			if err := writeProposalContent(dests[i], []byte(f.Content)); err != nil {
				return domain.Error(err, "write proposal file failed")
			}
		}

		if _, err := workrepo.CommitPathspec(ctx, workDir, message, pathspec); err != nil {
			return err
		}

		head, err := gitutil.RunOutput(ctx, workDir, "rev-parse", "HEAD")
		if err != nil {
			return domain.Error(err, "read HEAD sha failed")
		}
		sha = head
		return nil
	})
	if err != nil {
		return "", err
	}
	return sha, nil
}
