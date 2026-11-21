package corpus

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"claude-memory-sync/internal/gitutil/gittest"
)

const validEntry = "---\n" +
	"name: foo\n" +
	"description: a test entry\n" +
	"metadata:\n" +
	"  type: fact\n" +
	"---\n" +
	"body\n"

func newCorpusWorkDir(t *testing.T) string {
	t.Helper()

	repoDir := filepath.Join(t.TempDir(), "canonical.git")
	gittest.InitBareRepo(t, repoDir)

	workDir := t.TempDir()
	gittest.RunGit(t, "", nil, "clone", repoDir, workDir)
	gittest.RunGit(t, workDir, nil, "config", "user.name", "memory-synthesizer")
	gittest.RunGit(t, workDir, nil, "config", "user.email", "memory-synthesizer@local")

	memoryDir := filepath.Join(workDir, "projects", "proj1", "memory")
	require.NoError(t, os.MkdirAll(memoryDir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(memoryDir, "foo.md"), []byte(validEntry+"needle here\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(memoryDir, "MEMORY.md"), []byte("# Memory index\n\n<!-- HOT:BEGIN -->\nhot digest line\n<!-- HOT:END -->\n"), 0o600))

	gittest.RunGit(t, workDir, nil, "add", "-A")
	gittest.RunGit(t, workDir, nil, "commit", "-m", "seed")
	gittest.RunGit(t, workDir, nil, "push", "origin", "main")

	return workDir
}

func TestListProjects(t *testing.T) {
	t.Run("lists project summaries with entry counts and last commit", func(t *testing.T) {
		workDir := newCorpusWorkDir(t)

		summaries, err := ListProjects(context.Background(), workDir)
		require.NoError(t, err)
		require.Len(t, summaries, 1)
		assert.Equal(t, "proj1", summaries[0].Key)
		assert.Equal(t, 1, summaries[0].Entries)
		assert.NotEmpty(t, summaries[0].LastCommit)
	})
}

func TestSearch(t *testing.T) {
	t.Run("finds case-insensitive substring matches", func(t *testing.T) {
		workDir := newCorpusWorkDir(t)

		hits, err := Search(workDir, "NEEDLE")
		require.NoError(t, err)
		require.Len(t, hits, 1)
		assert.Equal(t, "proj1", hits[0].Project)
		assert.Equal(t, "foo.md", hits[0].File)
		assert.Contains(t, hits[0].Text, "needle")
	})

	t.Run("no matches returns empty slice", func(t *testing.T) {
		workDir := newCorpusWorkDir(t)

		hits, err := Search(workDir, "nonexistent-term")
		require.NoError(t, err)
		assert.Empty(t, hits)
	})
}

func TestReadEntry(t *testing.T) {
	t.Run("reads default MEMORY.md when file is omitted", func(t *testing.T) {
		workDir := newCorpusWorkDir(t)

		var buf bytes.Buffer
		err := ReadEntry(&buf, workDir, "proj1", "MEMORY.md")
		require.NoError(t, err)
		assert.Contains(t, buf.String(), "HOT:BEGIN")
	})

	t.Run("rejects unsafe file names", func(t *testing.T) {
		workDir := newCorpusWorkDir(t)

		var buf bytes.Buffer
		err := ReadEntry(&buf, workDir, "proj1", "../../../etc/passwd")
		require.Error(t, err)
		assert.Empty(t, buf.String())
	})
}

func TestWriteDigest(t *testing.T) {
	t.Run("extracts content between hot markers", func(t *testing.T) {
		workDir := newCorpusWorkDir(t)

		var buf bytes.Buffer
		err := WriteDigest(&buf, workDir, "proj1")
		require.NoError(t, err)
		assert.Equal(t, "hot digest line\n", buf.String())
	})

	t.Run("missing index produces no output", func(t *testing.T) {
		workDir := t.TempDir()

		var buf bytes.Buffer
		err := WriteDigest(&buf, workDir, "missing-project")
		require.NoError(t, err)
		assert.Empty(t, buf.String())
	})
}
