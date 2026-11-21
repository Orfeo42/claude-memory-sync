package intake

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"claude-memory-sync/internal/gitutil/gittest"
)

func TestAdvanceMarker(t *testing.T) {
	t.Run("advances the intake marker ref to the given sha", func(t *testing.T) {
		reposDir := filepath.Join(t.TempDir(), "repos")
		clientRepo := filepath.Join(reposDir, "clients", "client-a.git")
		gittest.InitBareRepo(t, clientRepo)

		head := gittest.CommitFiles(t, clientRepo, map[string]string{
			"global/CLAUDE.md": "v1",
		}, "seed", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))

		err := AdvanceMarker(context.Background(), reposDir, "client-a", head)
		require.NoError(t, err)

		marker := gittest.RunGit(t, "", nil, "-C", clientRepo, "rev-parse", "refs/intake/last-processed")
		assert.Contains(t, marker, head)
	})
}
