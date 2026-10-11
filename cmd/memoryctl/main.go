package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"

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
		return ensureWork(ctx, *reposDir, *workDir)
	})
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

	return syncWork(ctx, workDir)
}

func syncWork(ctx context.Context, workDir string) error {
	if err := gitutil.Run(ctx, workDir, "fetch", "origin"); err != nil {
		return domain.Error(err, "fetch origin failed")
	}

	behind, err := gitutil.IsAncestor(ctx, workDir, "HEAD", "origin/main")
	if err != nil {
		return domain.Error(err, "check work dir behind origin failed")
	}
	if behind {
		if err := gitutil.Run(ctx, workDir, "reset", "--hard", "origin/main"); err != nil {
			return domain.Error(err, "reset to origin/main failed")
		}
		return nil
	}

	ahead, err := gitutil.IsAncestor(ctx, workDir, "origin/main", "HEAD")
	if err != nil {
		return domain.Error(err, "check work dir ahead of origin failed")
	}
	if !ahead {
		head, err := gitutil.RunOutput(ctx, workDir, "rev-parse", "HEAD")
		if err != nil {
			return domain.Error(err, "resolve HEAD failed")
		}
		origin, err := gitutil.RunOutput(ctx, workDir, "rev-parse", "origin/main")
		if err != nil {
			return domain.Error(err, "resolve origin/main failed")
		}
		return domain.Error(domain.ErrConflict, "work dir diverged from origin/main",
			slog.String("head", head), slog.String("origin", origin))
	}

	return keepUnpushed(ctx, workDir)
}

func keepUnpushed(ctx context.Context, workDir string) error {
	count, err := gitutil.RunOutput(ctx, workDir, "rev-list", "--count", "origin/main..HEAD")
	if err != nil {
		return domain.Error(err, "count unpushed commits failed")
	}
	unpushed, err := strconv.Atoi(count)
	if err != nil {
		return domain.Error(err, "parse unpushed commit count failed", slog.String("count", count))
	}
	slog.WarnContext(ctx, "work dir ahead of origin, keeping unpushed commits", slog.Int("unpushed_commits", unpushed))

	if err := gitutil.Run(ctx, workDir, "reset", "--hard", "HEAD"); err != nil {
		return domain.Error(err, "reset to HEAD failed")
	}
	if err := gitutil.Run(ctx, workDir, "clean", "-fd"); err != nil {
		return domain.Error(err, "clean work dir failed")
	}

	return nil
}
