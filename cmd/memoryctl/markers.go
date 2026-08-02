package main

import (
	"context"
	"flag"
	"path/filepath"

	"claude-memory-sync/internal/domain"
	"claude-memory-sync/internal/gitutil"
)

func cmdMarkersAdvance(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("markers-advance", flag.ExitOnError)
	reposDir := fs.String("repos-dir", "", "repos directory")
	client := fs.String("client", "", "client id")
	to := fs.String("to", "", "target sha")
	if err := fs.Parse(args); err != nil {
		return domain.Error(err, "parse markers-advance flags failed")
	}

	return advanceMarker(ctx, *reposDir, *client, *to)
}

func advanceMarker(ctx context.Context, reposDir, client, to string) error {
	repoDir := filepath.Join(reposDir, "clients", client+".git")

	if err := gitutil.Run(ctx, repoDir, "update-ref", "refs/intake/last-processed", to); err != nil {
		return domain.Error(err, "update marker ref failed")
	}
	return nil
}
