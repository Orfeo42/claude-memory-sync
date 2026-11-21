package main

import (
	"context"
	"encoding/json"
	"flag"
	"os"

	"claude-memory-sync/internal/domain"
	"claude-memory-sync/internal/workrepo"
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

	committed, err := workrepo.Commit(ctx, workrepo.CommitOptions{WorkDir: *workDir, Lock: *lock, Message: *message, Pathspec: *pathspec})
	if err != nil {
		return err
	}

	if err := json.NewEncoder(os.Stdout).Encode(commitResult{Committed: committed}); err != nil {
		return domain.Error(err, "encode commit result failed")
	}
	return nil
}

func cmdPush(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("push", flag.ExitOnError)
	workDir := fs.String("work-dir", "", "work directory")
	lock := fs.String("lock", "", "lock file path")
	if err := fs.Parse(args); err != nil {
		return domain.Error(err, "parse push flags failed")
	}

	return workrepo.Push(ctx, *workDir, *lock)
}
