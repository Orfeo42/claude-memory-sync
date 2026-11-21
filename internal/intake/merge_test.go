package intake

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"claude-memory-sync/internal/gitutil/gittest"
)

func TestMergeConfig(t *testing.T) {
	t.Run("newer candidate wins by timestamp", func(t *testing.T) {
		reposDir := filepath.Join(t.TempDir(), "repos")
		workDir := t.TempDir()

		repoA := filepath.Join(reposDir, "clients", "client-a.git")
		gittest.InitBareRepo(t, repoA)
		headA := gittest.CommitFiles(t, repoA, map[string]string{"global/rules/foo.md": "from a"}, "seed", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))

		repoB := filepath.Join(reposDir, "clients", "client-b.git")
		gittest.InitBareRepo(t, repoB)
		headB := gittest.CommitFiles(t, repoB, map[string]string{"global/rules/foo.md": "from b"}, "seed", time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC))

		scan := ScanResult{Clients: []ClientScan{
			{
				ID:   "client-a",
				Head: headA,
				Config: []ConfigCandidate{
					{Path: "global/rules/foo.md", Timestamp: 1000, Deleted: false},
				},
			},
			{
				ID:   "client-b",
				Head: headB,
				Config: []ConfigCandidate{
					{Path: "global/rules/foo.md", Timestamp: 2000, Deleted: false},
				},
			},
		}}

		result, err := MergeConfig(context.Background(), reposDir, workDir, scan)
		require.NoError(t, err)
		assert.Equal(t, []string{"global/rules/foo.md"}, result.Changed)
		assert.Empty(t, result.Skipped)

		content, err := os.ReadFile(filepath.Join(workDir, "global/rules/foo.md"))
		require.NoError(t, err)
		assert.Equal(t, "from b", string(content))
	})

	t.Run("deleted winner removes existing file", func(t *testing.T) {
		reposDir := filepath.Join(t.TempDir(), "repos")
		workDir := t.TempDir()

		destPath := filepath.Join(workDir, "global", "rules", "bar.md")
		require.NoError(t, os.MkdirAll(filepath.Dir(destPath), 0o750))
		require.NoError(t, os.WriteFile(destPath, []byte("existing"), 0o600))

		scan := ScanResult{Clients: []ClientScan{
			{
				ID:   "client-a",
				Head: "deadbeef",
				Config: []ConfigCandidate{
					{Path: "global/rules/bar.md", Timestamp: 1000, Deleted: true},
				},
			},
		}}

		result, err := MergeConfig(context.Background(), reposDir, workDir, scan)
		require.NoError(t, err)
		assert.Equal(t, []string{"global/rules/bar.md"}, result.Changed)
		assert.Empty(t, result.Skipped)

		_, statErr := os.Stat(destPath)
		assert.True(t, os.IsNotExist(statErr))
	})

	t.Run("secret content is skipped", func(t *testing.T) {
		reposDir := filepath.Join(t.TempDir(), "repos")
		workDir := t.TempDir()

		repoA := filepath.Join(reposDir, "clients", "client-a.git")
		gittest.InitBareRepo(t, repoA)
		headA := gittest.CommitFiles(t, repoA, map[string]string{
			"global/rules/secret.md": "token: abcdefghijklmnop1234",
		}, "seed", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))

		scan := ScanResult{Clients: []ClientScan{
			{
				ID:   "client-a",
				Head: headA,
				Config: []ConfigCandidate{
					{Path: "global/rules/secret.md", Timestamp: 1000, Deleted: false},
				},
			},
		}}

		result, err := MergeConfig(context.Background(), reposDir, workDir, scan)
		require.NoError(t, err)
		assert.Empty(t, result.Changed)
		require.Len(t, result.Skipped, 1)
		assert.Equal(t, "global/rules/secret.md", result.Skipped[0].Path)
		assert.Equal(t, "secret", result.Skipped[0].Reason)

		_, statErr := os.Stat(filepath.Join(workDir, "global/rules/secret.md"))
		assert.True(t, os.IsNotExist(statErr))
	})
}
