package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
		initBareRepo(t, clientRepo)
		head := commitFiles(t, clientRepo, map[string]string{
			"projects/proj1/memory/candidate.md": "candidate content",
		}, "seed", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))

		scan := scanResult{Clients: []clientScan{
			{
				ID:   "client-a",
				Head: head,
				Memory: []memoryCandidate{
					{Project: "proj1", Path: "projects/proj1/memory/candidate.md"},
				},
			},
		}}

		var buf bytes.Buffer
		err := writeContext(context.Background(), &buf, reposDir, workDir, "proj1", scan)
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
