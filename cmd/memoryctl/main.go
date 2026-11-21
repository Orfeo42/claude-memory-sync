package main

import (
	"context"
	"flag"
	"log/slog"
	"os"

	"claude-memory-sync/internal/domain"
	"claude-memory-sync/internal/gitutil"
	"claude-memory-sync/internal/workrepo"
)

type cliError string

func (e cliError) Error() string { return string(e) }

const errUnknownSubcommand cliError = "unknown subcommand"

type subcommandFunc func(ctx context.Context, args []string) error

func subcommands() map[string]subcommandFunc {
	return map[string]subcommandFunc{
		"ensure-work":     cmdEnsureWork,
		"clients-scan":    cmdClientsScan,
		"config-merge":    cmdConfigMerge,
		"context":         cmdContext,
		"ops-apply":       cmdOpsApply,
		"commit":          cmdCommit,
		"push":            cmdPush,
		"markers-advance": cmdMarkersAdvance,
		"hotcache-write":  cmdHotcacheWrite,
		"proposal-apply":  cmdProposalApply,
		"corpus":          cmdCorpus,
		"proposal":        cmdProposal,
	}
}

func run(ctx context.Context, args []string) error {
	if len(args) < 1 {
		return domain.Error(errUnknownSubcommand, "subcommand required")
	}

	handler, ok := subcommands()[args[0]]
	if !ok {
		return domain.Error(errUnknownSubcommand, "unrecognized subcommand")
	}

	return handler(ctx, args[1:])
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	slog.SetDefault(logger)

	ctx := context.Background()

	if err := run(ctx, os.Args[1:]); err != nil {
		domain.LogError(ctx, "memoryctl command failed", err)
		os.Exit(1)
	}
}

func cmdEnsureWork(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("ensure-work", flag.ExitOnError)
	reposDir := fs.String("repos-dir", "", "repos directory")
	workDir := fs.String("work-dir", "", "work directory")
	lock := fs.String("lock", "", "canonical lock file path")
	if err := fs.Parse(args); err != nil {
		return domain.Error(err, "parse ensure-work flags failed")
	}
	if *lock == "" {
		return domain.Error(domain.ErrValidation, "lock flag is required")
	}

	return gitutil.WithFileLock(*lock, func() error {
		return workrepo.EnsureWork(ctx, *reposDir, *workDir)
	})
}
