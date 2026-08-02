package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func encodeScan(t *testing.T, scan scanResult) *bytes.Reader {
	t.Helper()
	data, err := json.Marshal(scan)
	require.NoError(t, err)
	return bytes.NewReader(data)
}

func TestMergeConfig(t *testing.T) {
	t.Run("newer candidate wins by timestamp", func(t *testing.T) {
		reposDir := filepath.Join(t.TempDir(), "repos")
		workDir := t.TempDir()

		repoA := filepath.Join(reposDir, "clients", "client-a.git")
		initBareRepo(t, repoA)
		headA := commitFiles(t, repoA, map[string]string{"global/rules/foo.md": "from a"}, "seed", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))

		repoB := filepath.Join(reposDir, "clients", "client-b.git")
		initBareRepo(t, repoB)
		headB := commitFiles(t, repoB, map[string]string{"global/rules/foo.md": "from b"}, "seed", time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC))

		scan := scanResult{Clients: []clientScan{
			{
				ID:   "client-a",
				Head: headA,
				Config: []configCandidate{
					{Path: "global/rules/foo.md", Timestamp: 1000, Deleted: false},
				},
			},
			{
				ID:   "client-b",
				Head: headB,
				Config: []configCandidate{
					{Path: "global/rules/foo.md", Timestamp: 2000, Deleted: false},
				},
			},
		}}

		result, err := mergeConfig(context.Background(), reposDir, workDir, encodeScan(t, scan))
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

		scan := scanResult{Clients: []clientScan{
			{
				ID:   "client-a",
				Head: "deadbeef",
				Config: []configCandidate{
					{Path: "global/rules/bar.md", Timestamp: 1000, Deleted: true},
				},
			},
		}}

		result, err := mergeConfig(context.Background(), reposDir, workDir, encodeScan(t, scan))
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
		initBareRepo(t, repoA)
		headA := commitFiles(t, repoA, map[string]string{
			"global/rules/secret.md": "token: abcdefghijklmnop1234",
		}, "seed", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))

		scan := scanResult{Clients: []clientScan{
			{
				ID:   "client-a",
				Head: headA,
				Config: []configCandidate{
					{Path: "global/rules/secret.md", Timestamp: 1000, Deleted: false},
				},
			},
		}}

		result, err := mergeConfig(context.Background(), reposDir, workDir, encodeScan(t, scan))
		require.NoError(t, err)
		assert.Empty(t, result.Changed)
		require.Len(t, result.Skipped, 1)
		assert.Equal(t, "global/rules/secret.md", result.Skipped[0].Path)
		assert.Equal(t, "secret", result.Skipped[0].Reason)

		_, statErr := os.Stat(filepath.Join(workDir, "global/rules/secret.md"))
		assert.True(t, os.IsNotExist(statErr))
	})
}
