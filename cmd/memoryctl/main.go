package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"path/filepath"

	"claude-memory-sync/internal/domain"
	"claude-memory-sync/internal/gitutil"
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
	if err := fs.Parse(args); err != nil {
		return domain.Error(err, "parse ensure-work flags failed")
	}

	return ensureWork(ctx, *reposDir, *workDir)
}

func ensureWork(ctx context.Context, reposDir, workDir string) error {
	if err := ensureWorkClone(ctx, reposDir, workDir); err != nil {
		return err
	}

	if err := gitutil.Run(ctx, workDir, "config", "user.name", "memory-synthesizer"); err != nil {
		return domain.Error(err, "set git user.name failed")
	}
	if err := gitutil.Run(ctx, workDir, "config", "user.email", "memory-synthesizer@local"); err != nil {
		return domain.Error(err, "set git user.email failed")
	}

	return nil
}

func ensureWorkClone(ctx context.Context, reposDir, workDir string) error {
	canonicalRepo := filepath.Join(reposDir, "canonical.git")

	if _, err := os.Stat(filepath.Join(workDir, ".git")); os.IsNotExist(err) {
		if err := gitutil.Run(ctx, "", "clone", canonicalRepo, workDir); err != nil {
			return domain.Error(err, "clone canonical repo failed")
		}
		return nil
	}

	if err := gitutil.Run(ctx, workDir, "fetch", "origin"); err != nil {
		return domain.Error(err, "fetch origin failed")
	}
	if err := gitutil.Run(ctx, workDir, "reset", "--hard", "origin/main"); err != nil {
		return domain.Error(err, "reset to origin/main failed")
	}

	return nil
}
