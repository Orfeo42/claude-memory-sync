package intake

import (
	"context"
	"path/filepath"

	"claude-memory-sync/internal/domain"
	"claude-memory-sync/internal/gitutil"
)

func AdvanceMarker(ctx context.Context, reposDir, client, to string) error {
	repoDir := filepath.Join(reposDir, "clients", client+".git")

	if err := gitutil.Run(ctx, repoDir, "update-ref", "refs/intake/last-processed", to); err != nil {
		return domain.Error(err, "update marker ref failed")
	}
	return nil
}
