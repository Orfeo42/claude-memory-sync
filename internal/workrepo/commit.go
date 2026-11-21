package workrepo

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"claude-memory-sync/internal/domain"
	"claude-memory-sync/internal/gitutil"
)

const (
	commitRetryAttempts = 3
	commitRetryDelay    = 2 * time.Second
)

type CommitOptions struct {
	WorkDir  string
	Lock     string
	Message  string
	Pathspec string
}

func Commit(ctx context.Context, opts CommitOptions) (bool, error) {
	committed := false

	err := gitutil.WithFileLock(opts.Lock, func() error {
		var err error
		committed, err = CommitPathspec(ctx, opts.WorkDir, opts.Message, opts.Pathspec)
		return err
	})

	return committed, err
}

func CommitPathspec(ctx context.Context, workDir, message, pathspec string) (bool, error) {
	if _, err := os.Stat(filepath.Join(workDir, pathspec)); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, domain.Error(err, "stat pathspec failed", slog.String("pathspec", pathspec))
	}

	if err := gitutil.Run(ctx, workDir, "add", "-A", "--", pathspec); err != nil {
		return false, domain.Error(err, "git add failed")
	}

	has, err := gitutil.HasStagedChanges(ctx, workDir)
	if err != nil {
		return false, domain.Error(err, "check staged changes failed")
	}
	if !has {
		return false, nil
	}

	if err := commitWithRetry(ctx, workDir, message, pathspec); err != nil {
		return false, err
	}
	return true, nil
}

func commitWithRetry(ctx context.Context, workDir, message, pathspec string) error {
	var lastErr error
	for range commitRetryAttempts {
		lastErr = gitutil.Run(ctx, workDir, "commit", "-m", message, "--", pathspec)
		if lastErr == nil {
			return nil
		}
		time.Sleep(commitRetryDelay)
	}
	return domain.Error(lastErr, "commit failed after retries")
}

func Push(ctx context.Context, workDir, lock string) error {
	return gitutil.WithFileLock(lock, func() error {
		if err := gitutil.Run(ctx, workDir, "push", "origin", "main"); err != nil {
			return domain.Error(err, "git push failed")
		}
		return nil
	})
}
