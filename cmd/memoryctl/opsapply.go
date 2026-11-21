package main

import (
	"context"
	"encoding/json"
	"flag"
	"os"

	"claude-memory-sync/internal/domain"
	"claude-memory-sync/internal/ops"
	"claude-memory-sync/internal/proposalapply"
)

func cmdOpsApply(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("ops-apply", flag.ExitOnError)
	memoryDir := fs.String("memory-dir", "", "memory directory")
	allowDelete := fs.Bool("allow-delete", false, "allow delete ops")
	proposalsDir := fs.String("proposals-dir", "", "proposals directory")
	sourceFlow := fs.String("source-flow", "", "source flow for API proposals")
	sourceRun := fs.String("source-run", "", "source run for API proposals")
	if err := fs.Parse(args); err != nil {
		return domain.Error(err, "parse ops-apply flags failed")
	}

	opts := ops.ApplyOptions{MemoryDir: *memoryDir, AllowDelete: *allowDelete, ProposalsDir: *proposalsDir}
	if *sourceFlow != "" || *sourceRun != "" {
		if *sourceFlow == "" || *sourceRun == "" {
			return domain.Error(domain.ErrValidation, "source-flow and source-run must be set together")
		}
		if *proposalsDir != "" {
			return domain.Error(domain.ErrValidation, "proposals-dir cannot be combined with source-flow and source-run")
		}

		pipelineClient, err := newPipelineClient()
		if err != nil {
			return err
		}
		opts.Submitter = proposalapply.NewSubmitter(pipelineClient, *sourceFlow, *sourceRun)
	}

	result, err := ops.Apply(ctx, opts, os.Stdin)
	if err != nil {
		return err
	}

	if err := json.NewEncoder(os.Stdout).Encode(result); err != nil {
		return domain.Error(err, "encode ops-apply result failed")
	}
	return nil
}
