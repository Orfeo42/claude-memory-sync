package workrepo

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"

	"claude-memory-sync/internal/domain"
	"claude-memory-sync/internal/gitutil"
)

func EnsureWork(ctx context.Context, reposDir, workDir string) error {
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
