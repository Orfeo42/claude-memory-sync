package main

import (
	"context"
	"encoding/json"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"time"

	"claude-memory-sync/internal/domain"
	"claude-memory-sync/internal/pipeline"
	"claude-memory-sync/internal/pipeline/client"
	"claude-memory-sync/internal/proposalapply"
)

const pipelineHTTPTimeout = 30 * time.Second

type reviseInput struct {
	Files []pipeline.File `json:"files"`
}

func newPipelineClient() (*client.Client, error) {
	cfg, err := client.ConfigFromEnv()
	if err != nil {
		return nil, err
	}
	return client.New(cfg, &http.Client{Timeout: pipelineHTTPTimeout}), nil
}

func proposalSubcommands() map[string]subcommandFunc {
	return map[string]subcommandFunc{
		"list":              cmdProposalList,
		"get":               cmdProposalGet,
		"rejections":        cmdProposalRejections,
		"decide":            cmdProposalDecide,
		"revise":            cmdProposalRevise,
		"retry":             cmdProposalRetry,
		"apply-approved":    cmdProposalApplyApproved,
		"expire":            cmdProposalExpire,
		"import":            cmdProposalImport,
		"import-rejections": cmdProposalImportRejections,
	}
}

func cmdProposal(ctx context.Context, args []string) error {
	if len(args) < 1 {
		return domain.Error(errUnknownSubcommand, "proposal subcommand required")
	}

	handler, ok := proposalSubcommands()[args[0]]
	if !ok {
		return domain.Error(errUnknownSubcommand, "unrecognized proposal subcommand")
	}

	return handler(ctx, args[1:])
}

func cmdProposalList(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("proposal list", flag.ExitOnError)
	statusFlag := fs.String("status", "", "status filter")
	kindFlag := fs.String("kind", "", "kind filter")
	limit := fs.Int("limit", 0, "max results")
	if err := fs.Parse(args); err != nil {
		return domain.Error(err, "parse proposal list flags failed")
	}

	params := client.ListParams{Limit: *limit}
	if *statusFlag != "" {
		status, err := pipeline.ParseStatus(*statusFlag)
		if err != nil {
			return err
		}
		params.Status = &status
	}
	if *kindFlag != "" {
		kind, err := pipeline.ParseKind(*kindFlag)
		if err != nil {
			return err
		}
		params.Kind = &kind
	}

	pipelineClient, err := newPipelineClient()
	if err != nil {
		return err
	}
	proposals, err := pipelineClient.List(ctx, params)
	if err != nil {
		return err
	}
	if err := json.NewEncoder(os.Stdout).Encode(proposals); err != nil {
		return domain.Error(err, "encode result failed")
	}
	return nil
}

func cmdProposalGet(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("proposal get", flag.ExitOnError)
	id := fs.String("id", "", "proposal id")
	if err := fs.Parse(args); err != nil {
		return domain.Error(err, "parse proposal get flags failed")
	}

	pipelineClient, err := newPipelineClient()
	if err != nil {
		return err
	}
	detail, err := pipelineClient.Get(ctx, *id)
	if err != nil {
		return err
	}
	if err := json.NewEncoder(os.Stdout).Encode(detail); err != nil {
		return domain.Error(err, "encode result failed")
	}
	return nil
}

func cmdProposalRejections(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("proposal rejections", flag.ExitOnError)
	kindFlag := fs.String("kind", "", "kind filter")
	if err := fs.Parse(args); err != nil {
		return domain.Error(err, "parse proposal rejections flags failed")
	}

	var kind *pipeline.Kind
	if *kindFlag != "" {
		parsed, err := pipeline.ParseKind(*kindFlag)
		if err != nil {
			return err
		}
		kind = &parsed
	}

	pipelineClient, err := newPipelineClient()
	if err != nil {
		return err
	}
	rejections, err := pipelineClient.Rejections(ctx, kind)
	if err != nil {
		return err
	}
	if err := json.NewEncoder(os.Stdout).Encode(rejections); err != nil {
		return domain.Error(err, "encode result failed")
	}
	return nil
}

func cmdProposalDecide(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("proposal decide", flag.ExitOnError)
	id := fs.String("id", "", "proposal id")
	decision := fs.String("decision", "", "approve or reject")
	actor := fs.String("actor", "", "actor")
	reason := fs.String("reason", "", "reason")
	if err := fs.Parse(args); err != nil {
		return domain.Error(err, "parse proposal decide flags failed")
	}

	pipelineClient, err := newPipelineClient()
	if err != nil {
		return err
	}
	proposal, err := pipelineClient.Decide(ctx, *id, client.DecideRequest{Decision: *decision, Actor: *actor, Reason: *reason})
	if err != nil {
		return err
	}
	if err := json.NewEncoder(os.Stdout).Encode(proposal); err != nil {
		return domain.Error(err, "encode result failed")
	}
	return nil
}

func cmdProposalRevise(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("proposal revise", flag.ExitOnError)
	id := fs.String("id", "", "proposal id")
	actor := fs.String("actor", "", "actor")
	if err := fs.Parse(args); err != nil {
		return domain.Error(err, "parse proposal revise flags failed")
	}

	var input reviseInput
	if err := json.NewDecoder(os.Stdin).Decode(&input); err != nil {
		return domain.Error(domain.ErrValidation, "decode revise input failed", slog.String("cause", err.Error()))
	}

	pipelineClient, err := newPipelineClient()
	if err != nil {
		return err
	}
	proposal, err := pipelineClient.Revise(ctx, *id, client.ReviseRequest{Actor: *actor, Files: input.Files})
	if err != nil {
		return err
	}
	if err := json.NewEncoder(os.Stdout).Encode(proposal); err != nil {
		return domain.Error(err, "encode result failed")
	}
	return nil
}

func cmdProposalRetry(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("proposal retry", flag.ExitOnError)
	id := fs.String("id", "", "proposal id")
	actor := fs.String("actor", "", "actor")
	if err := fs.Parse(args); err != nil {
		return domain.Error(err, "parse proposal retry flags failed")
	}

	pipelineClient, err := newPipelineClient()
	if err != nil {
		return err
	}
	proposal, err := pipelineClient.Retry(ctx, *id, *actor)
	if err != nil {
		return err
	}
	if err := json.NewEncoder(os.Stdout).Encode(proposal); err != nil {
		return domain.Error(err, "encode result failed")
	}
	return nil
}

func cmdProposalExpire(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("proposal expire", flag.ExitOnError)
	olderThan := fs.Duration("older-than", 0, "expire pending proposals older than this duration")
	actor := fs.String("actor", "", "actor")
	if err := fs.Parse(args); err != nil {
		return domain.Error(err, "parse proposal expire flags failed")
	}

	pipelineClient, err := newPipelineClient()
	if err != nil {
		return err
	}
	result, err := proposalapply.Expire(ctx, pipelineClient, *actor, *olderThan)
	if err != nil {
		return err
	}
	if err := json.NewEncoder(os.Stdout).Encode(result); err != nil {
		return domain.Error(err, "encode result failed")
	}
	return nil
}

func cmdProposalApplyApproved(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("proposal apply-approved", flag.ExitOnError)
	workDir := fs.String("work-dir", "", "work directory")
	lock := fs.String("lock", "", "lock file path")
	worker := fs.String("worker", "", "worker name")
	if err := fs.Parse(args); err != nil {
		return domain.Error(err, "parse proposal apply-approved flags failed")
	}

	pipelineClient, err := newPipelineClient()
	if err != nil {
		return err
	}

	result, applyErr := proposalapply.ApplyApproved(ctx, pipelineClient, proposalapply.ApplyApprovedOptions{WorkDir: *workDir, Lock: *lock, Worker: *worker})
	if err := json.NewEncoder(os.Stdout).Encode(result); err != nil {
		return domain.Error(err, "encode result failed")
	}
	return applyErr
}

func cmdProposalImport(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("proposal import", flag.ExitOnError)
	dir := fs.String("dir", "", "staging directory")
	sourceFlow := fs.String("source-flow", "", "source flow")
	if err := fs.Parse(args); err != nil {
		return domain.Error(err, "parse proposal import flags failed")
	}

	pipelineClient, err := newPipelineClient()
	if err != nil {
		return err
	}

	outcome, err := proposalapply.Import(ctx, pipelineClient, *dir, *sourceFlow)
	if err != nil {
		return err
	}
	if err := json.NewEncoder(os.Stdout).Encode(outcome); err != nil {
		return domain.Error(err, "encode result failed")
	}
	return nil
}

func cmdProposalImportRejections(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("proposal import-rejections", flag.ExitOnError)
	file := fs.String("file", "", "legacy rejected-names file")
	actor := fs.String("actor", "", "actor")
	if err := fs.Parse(args); err != nil {
		return domain.Error(err, "parse proposal import-rejections flags failed")
	}

	pipelineClient, err := newPipelineClient()
	if err != nil {
		return err
	}

	result, err := proposalapply.ImportRejections(ctx, pipelineClient, proposalapply.ImportRejectionsOptions{File: *file, Actor: *actor})
	if err != nil {
		return err
	}
	if err := json.NewEncoder(os.Stdout).Encode(result); err != nil {
		return domain.Error(err, "encode result failed")
	}
	return nil
}

func cmdProposalApply(_ context.Context, args []string) error {
	fs := flag.NewFlagSet("proposal-apply", flag.ExitOnError)
	workDir := fs.String("work-dir", "", "work directory")
	file := fs.String("file", "", "flattened proposal file")
	if err := fs.Parse(args); err != nil {
		return domain.Error(err, "parse proposal-apply flags failed")
	}

	return proposalapply.ApplyFile(*workDir, *file)
}
