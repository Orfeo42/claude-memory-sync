package proposalapply

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"claude-memory-sync/internal/domain"
	"claude-memory-sync/internal/ops"
	"claude-memory-sync/internal/pipeline"
	"claude-memory-sync/internal/pipeline/client"
)

const importedRejectionReason = "imported from taskmine-rejected"

type ImportRejectionsOptions struct {
	File  string
	Actor string
}

type ImportRejectionsResult struct {
	Imported int `json:"imported"`
}

func Import(ctx context.Context, p Pipeline, dir, sourceFlow string) (ops.SubmitOutcome, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ops.SubmitOutcome{}, domain.Error(err, "read staging directory failed")
	}

	files := make([]pipeline.File, 0, len(entries))
	rejected := make([]ops.RejectedOp, 0)
	for _, entry := range entries {
		if entry.IsDir() || strings.HasSuffix(entry.Name(), ".path") {
			continue
		}

		flattened := filepath.Join(dir, entry.Name())
		target, err := resolveProposalRealPath(flattened)
		if err != nil {
			return ops.SubmitOutcome{}, err
		}
		if err := pipeline.ValidateTarget(target); err != nil {
			rejected = append(rejected, ops.RejectedOp{Name: entry.Name(), Reason: ops.ReasonInvalidProposalPath})
			continue
		}

		content, err := os.ReadFile(flattened)
		if err != nil {
			return ops.SubmitOutcome{}, domain.Error(err, "read staged proposal file failed")
		}
		files = append(files, pipeline.File{TargetPath: target, Content: string(content)})
	}

	submitter := NewSubmitter(p, sourceFlow, filepath.Base(filepath.Clean(dir)))
	outcome, err := submitter.Submit(ctx, files)
	if err != nil {
		return ops.SubmitOutcome{}, err
	}

	outcome.Rejected = append(rejected, outcome.Rejected...)
	return outcome, nil
}

func ImportRejections(ctx context.Context, p Pipeline, opts ImportRejectionsOptions) (ImportRejectionsResult, error) {
	if opts.File == "" {
		return ImportRejectionsResult{}, domain.Error(domain.ErrValidation, "file required")
	}

	data, err := os.ReadFile(opts.File)
	if err != nil {
		return ImportRejectionsResult{}, domain.Error(domain.ErrValidation, "read rejected-names file failed", slog.String("cause", err.Error()))
	}

	reason := importedRejectionReason
	imported := 0
	for _, name := range legacyRejectionNames(string(data)) {
		kind := pipeline.KindSkill
		if strings.HasSuffix(name, ".md") {
			kind = pipeline.KindOf(name)
		}
		if err := p.AddRejection(ctx, client.AddRejectionRequest{Kind: kind, Name: name, Actor: opts.Actor, Reason: &reason}); err != nil {
			return ImportRejectionsResult{}, err
		}
		imported++
	}

	return ImportRejectionsResult{Imported: imported}, nil
}

func legacyRejectionNames(data string) []string {
	seen := map[string]bool{}
	names := make([]string, 0)
	for _, line := range strings.Split(data, "\n") {
		name := strings.TrimSpace(line)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		names = append(names, name)
	}
	return names
}
