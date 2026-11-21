package ops

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"claude-memory-sync/internal/domain"
	"claude-memory-sync/internal/guardrails"
	"claude-memory-sync/internal/pipeline"
)

const (
	ReasonInvalidProposalPath = "invalid proposal path"

	reasonDeletesNotAllowed   = "deletes not allowed"
	reasonNoSuchEntry         = "no such entry"
	reasonProposalsNotAllowed = "proposals not allowed"
)

type ApplyOptions struct {
	MemoryDir    string
	AllowDelete  bool
	ProposalsDir string
	Submitter    ProposalSubmitter
}

type SubmitOutcome struct {
	Created  []string     `json:"created"`
	Rejected []RejectedOp `json:"rejected"`
}

//go:generate go tool moq -out submitter_mock.go . ProposalSubmitter:MockProposalSubmitter
type ProposalSubmitter interface {
	Submit(ctx context.Context, files []pipeline.File) (SubmitOutcome, error)
}

type RejectedOp struct {
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

type ApplyResult struct {
	Applied     []string     `json:"applied"`
	Deleted     []string     `json:"deleted"`
	Proposals   []string     `json:"proposals"`
	ProposalIDs []string     `json:"proposal_ids"`
	Rejected    []RejectedOp `json:"rejected"`
	Nothing     bool         `json:"nothing"`
}

func Apply(ctx context.Context, opts ApplyOptions, in io.Reader) (ApplyResult, error) {
	parsed, err := Parse(in)
	if err != nil {
		return ApplyResult{}, domain.Error(err, "parse ops output failed")
	}

	result := ApplyResult{
		Applied:     make([]string, 0, len(parsed.Files)),
		Deleted:     make([]string, 0, len(parsed.Deletes)),
		Proposals:   make([]string, 0, len(parsed.Proposals)),
		ProposalIDs: make([]string, 0, len(parsed.Proposals)),
		Rejected:    make([]RejectedOp, 0),
	}

	if parsed.Nothing {
		result.Nothing = true
		return result, nil
	}

	if len(parsed.Files) > 0 {
		if err := os.MkdirAll(opts.MemoryDir, 0o750); err != nil {
			return ApplyResult{}, domain.Error(err, "create memory dir failed", slog.String("memory_dir", opts.MemoryDir))
		}
	}

	applyFileOps(opts.MemoryDir, parsed.Files, &result)
	applyDeleteOps(opts.MemoryDir, opts.AllowDelete, parsed.Deletes, &result)

	if opts.Submitter != nil {
		if err := submitProposalOps(ctx, opts.Submitter, parsed.Proposals, &result); err != nil {
			return ApplyResult{}, err
		}
	} else {
		applyProposalOps(opts.ProposalsDir, parsed.Proposals, &result)
	}

	if len(result.Applied) > 0 || len(result.Deleted) > 0 {
		if err := guardrails.RebuildIndex(opts.MemoryDir); err != nil {
			return ApplyResult{}, domain.Error(err, "rebuild memory index failed")
		}
	}

	return result, nil
}

func submitProposalOps(ctx context.Context, submitter ProposalSubmitter, proposals []FileOp, result *ApplyResult) error {
	files := make([]pipeline.File, 0, len(proposals))
	for _, p := range proposals {
		if err := pipeline.ValidateTarget(p.Name); err != nil {
			reject(result, p.Name, ReasonInvalidProposalPath)
			continue
		}
		if err := scanContent(p.Content); err != nil {
			reject(result, p.Name, err.Error())
			continue
		}
		files = append(files, pipeline.File{TargetPath: p.Name, Content: string(p.Content)})
	}

	outcome, err := submitter.Submit(ctx, files)
	if err != nil {
		return err
	}

	result.ProposalIDs = append(result.ProposalIDs, outcome.Created...)
	result.Rejected = append(result.Rejected, outcome.Rejected...)
	return nil
}

func reject(result *ApplyResult, name, reason string) {
	result.Rejected = append(result.Rejected, RejectedOp{Name: name, Reason: reason})
}

func applyFileOps(memoryDir string, files []FileOp, result *ApplyResult) {
	for _, f := range files {
		name, err := guardrails.NormalizeFilename(f.Name)
		if err != nil {
			reject(result, f.Name, err.Error())
			continue
		}
		if err := guardrails.ValidateFrontmatter(f.Content); err != nil {
			reject(result, name, err.Error())
			continue
		}
		if err := scanContent(f.Content); err != nil {
			reject(result, name, err.Error())
			continue
		}
		if err := os.WriteFile(filepath.Join(memoryDir, name), f.Content, 0o600); err != nil {
			reject(result, name, err.Error())
			continue
		}
		result.Applied = append(result.Applied, name)
	}
}

func applyDeleteOps(memoryDir string, allowDelete bool, deletes []string, result *ApplyResult) {
	for _, raw := range deletes {
		name, err := guardrails.NormalizeFilename(raw)
		if err != nil {
			reject(result, raw, err.Error())
			continue
		}
		if !allowDelete {
			reject(result, name, reasonDeletesNotAllowed)
			continue
		}

		path := filepath.Join(memoryDir, name)
		if _, statErr := os.Stat(path); statErr != nil {
			reject(result, name, reasonNoSuchEntry)
			continue
		}
		if err := os.Remove(path); err != nil {
			reject(result, name, err.Error())
			continue
		}
		result.Deleted = append(result.Deleted, name)
	}
}

func applyProposalOps(proposalsDir string, proposals []FileOp, result *ApplyResult) {
	for _, p := range proposals {
		if proposalsDir == "" {
			reject(result, p.Name, reasonProposalsNotAllowed)
			continue
		}
		if err := pipeline.ValidateTarget(p.Name); err != nil {
			reject(result, p.Name, ReasonInvalidProposalPath)
			continue
		}
		if err := scanContent(p.Content); err != nil {
			reject(result, p.Name, err.Error())
			continue
		}

		flattened := strings.ReplaceAll(p.Name, "/", "__")
		if err := writeProposalFiles(proposalsDir, flattened, p); err != nil {
			reject(result, p.Name, err.Error())
			continue
		}
		result.Proposals = append(result.Proposals, flattened)
	}
}

func writeProposalFiles(proposalsDir, flattened string, p FileOp) error {
	if err := os.MkdirAll(proposalsDir, 0o750); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(proposalsDir, flattened), p.Content, 0o600); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(proposalsDir, flattened+".path"), []byte(p.Name), 0o600)
}

func scanContent(content []byte) error {
	if err := guardrails.ScanSecrets(content); err != nil {
		return err
	}
	return guardrails.ScanHidden(content)
}
