package corpus

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"claude-memory-sync/internal/gitutil/gittest"
	"claude-memory-sync/internal/intake"
)

func TestWriteContext(t *testing.T) {
	t.Run("renders index, entries, and candidates from contributing clients", func(t *testing.T) {
		reposDir := filepath.Join(t.TempDir(), "repos")
		workDir := t.TempDir()

		memoryDir := filepath.Join(workDir, "projects", "proj1", "memory")
		require.NoError(t, os.MkdirAll(memoryDir, 0o750))
		require.NoError(t, os.WriteFile(filepath.Join(memoryDir, "MEMORY.md"), []byte("# Memory index\n"), 0o600))
		require.NoError(t, os.WriteFile(filepath.Join(memoryDir, "existing.md"), []byte(validEntry), 0o600))

		clientRepo := filepath.Join(reposDir, "clients", "client-a.git")
		gittest.InitBareRepo(t, clientRepo)
		head := gittest.CommitFiles(t, clientRepo, map[string]string{
			"projects/proj1/memory/candidate.md": "candidate content",
		}, "seed", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))

		scan := intake.ScanResult{Clients: []intake.ClientScan{
			{
				ID:   "client-a",
				Head: head,
				Memory: []intake.MemoryCandidate{
					{Project: "proj1", Path: "projects/proj1/memory/candidate.md"},
				},
			},
		}}

		var buf bytes.Buffer
		err := WriteContext(context.Background(), &buf, reposDir, workDir, "proj1", scan)
		require.NoError(t, err)

		output := buf.String()
		assert.Contains(t, output, "INDEX:")
		assert.Contains(t, output, "ENTRIES:")
		assert.Contains(t, output, "--- existing.md ---")
		assert.Contains(t, output, "CANDIDATES (from machine client-a):")
		assert.Contains(t, output, "--- candidate.md ---")
		assert.Contains(t, output, "candidate content")
	})
}
