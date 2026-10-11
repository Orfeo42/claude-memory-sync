package main

import (
	"context"
	"encoding/json"
	"flag"
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

type commitResult struct {
	Committed bool `json:"committed"`
}

func cmdCommit(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("commit", flag.ExitOnError)
	workDir := fs.String("work-dir", "", "work directory")
	lock := fs.String("lock", "", "lock file path")
	message := fs.String("message", "", "commit message")
	pathspec := fs.String("pathspec", "", "pathspec")
	if err := fs.Parse(args); err != nil {
		return domain.Error(err, "parse commit flags failed")
	}

	committed, err := runCommit(ctx, *workDir, *lock, *message, *pathspec)
	if err != nil {
		return err
	}

	if err := json.NewEncoder(os.Stdout).Encode(commitResult{Committed: committed}); err != nil {
		return domain.Error(err, "encode commit result failed")
	}
	return nil
}

func runCommit(ctx context.Context, workDir, lock, message, pathspec string) (bool, error) {
	committed := false

	err := gitutil.WithFileLock(lock, func() error {
		var err error
		committed, err = commitPathspec(ctx, workDir, message, pathspec)
		return err
	})

	return committed, err
}

func commitPathspec(ctx context.Context, workDir, message, pathspec string) (bool, error) {
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

func cmdPush(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("push", flag.ExitOnError)
	workDir := fs.String("work-dir", "", "work directory")
	lock := fs.String("lock", "", "lock file path")
	if err := fs.Parse(args); err != nil {
		return domain.Error(err, "parse push flags failed")
	}

	return pushWithLock(ctx, *workDir, *lock)
}

func pushWithLock(ctx context.Context, workDir, lock string) error {
	return gitutil.WithFileLock(lock, func() error {
		if err := gitutil.Run(ctx, workDir, "push", "origin", "main"); err != nil {
			return domain.Error(err, "git push failed")
		}
		return nil
	})
}
