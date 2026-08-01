package syncer

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func newTestConfig(t *testing.T, clientID, slugPrefix string) (Config, string) {
	t.Helper()

	base := t.TempDir()
	clientRepo := filepath.Join(base, "git", "clients", clientID+".git")
	require.NoError(t, os.MkdirAll(filepath.Dir(clientRepo), 0o750))
	runGit(t, "", "init", "--bare", "--initial-branch=main", clientRepo)

	canonicalRepo := filepath.Join(base, "git", "canonical.git")
	require.NoError(t, os.MkdirAll(filepath.Dir(canonicalRepo), 0o750))
	runGit(t, "", "init", "--bare", "--initial-branch=main", canonicalRepo)

	cfg := Config{
		ServerURL:  "file://" + base,
		Token:      "test-token",
		ClientID:   clientID,
		SlugPrefix: slugPrefix,
		ClaudeDir:  t.TempDir(),
		StateDir:   t.TempDir(),
	}

	return cfg, base
}

func writeClaudeMD(t *testing.T, claudeDir, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(claudeDir, "CLAUDE.md"), []byte(content), 0o644))
}

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "git %v: %s", args, out)
	return string(out)
}

func bareGitDir(base, name string) string {
	return filepath.Join(base, "git", name)
}

func clientBareDir(base, clientID string) string {
	return bareGitDir(base, filepath.Join("clients", clientID+".git"))
}

func canonicalBareDir(base string) string {
	return bareGitDir(base, "canonical.git")
}

func bareCommitCount(t *testing.T, gitDir string) int {
	t.Helper()
	out := runGit(t, "", "--git-dir", gitDir, "rev-list", "--count", "main")
	count, err := strconv.Atoi(strings.TrimSpace(out))
	require.NoError(t, err)
	return count
}

func bareLsTree(t *testing.T, gitDir string) []string {
	t.Helper()
	out := runGit(t, "", "--git-dir", gitDir, "ls-tree", "-r", "--name-only", "main")
	trimmed := strings.TrimSpace(out)
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "\n")
}

func bareHeadDiffPaths(t *testing.T, gitDir string) []string {
	t.Helper()
	out := runGit(t, "", "--git-dir", gitDir, "diff-tree", "--no-commit-id", "--name-only", "-r", "HEAD")
	trimmed := strings.TrimSpace(out)
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "\n")
}

func seedBareFile(t *testing.T, gitDir, relPath, content string) {
	t.Helper()
	scratch := t.TempDir()
	runGit(t, "", "clone", "file://"+gitDir, scratch)

	fullPath := filepath.Join(scratch, filepath.FromSlash(relPath))
	require.NoError(t, os.MkdirAll(filepath.Dir(fullPath), 0o750))
	require.NoError(t, os.WriteFile(fullPath, []byte(content), 0o600))

	runGit(t, scratch, "config", "user.name", "test-seed")
	runGit(t, scratch, "config", "user.email", "test-seed@test.local")
	runGit(t, scratch, "add", "-A")
	runGit(t, scratch, "commit", "-m", "seed")
	runGit(t, scratch, "push", "origin", "main")
}

func removeBareFile(t *testing.T, gitDir, relPath string) {
	t.Helper()
	scratch := t.TempDir()
	runGit(t, "", "clone", "file://"+gitDir, scratch)

	runGit(t, scratch, "config", "user.name", "test-seed")
	runGit(t, scratch, "config", "user.email", "test-seed@test.local")
	runGit(t, scratch, "rm", filepath.FromSlash(relPath))
	runGit(t, scratch, "commit", "-m", "remove")
	runGit(t, scratch, "push", "origin", "main")
}
