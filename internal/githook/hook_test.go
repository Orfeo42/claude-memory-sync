package githook_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"claude-memory-sync/internal/githook"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const zeroSHA = "0000000000000000000000000000000000000000"

func execGit(t *testing.T, dir string, args ...string) string {
	t.Helper()

	cmd := exec.Command("git", args...)
	cmd.Dir = dir

	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v: %s", args, out)

	return strings.TrimSpace(string(out))
}

func newBareRepo(t *testing.T) string {
	t.Helper()

	root := t.TempDir()
	execGit(t, root, "init", "--bare", "--initial-branch=main", "repo.git")

	return filepath.Join(root, "repo.git")
}

func newWorkTree(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	execGit(t, dir, "init", "--initial-branch=main")
	execGit(t, dir, "config", "user.email", "test@example.com")
	execGit(t, dir, "config", "user.name", "Test User")

	return dir
}

func commitFile(t *testing.T, workDir, relPath, content string) {
	t.Helper()

	fullPath := filepath.Join(workDir, relPath)
	require.NoError(t, os.MkdirAll(filepath.Dir(fullPath), 0o755))
	require.NoError(t, os.WriteFile(fullPath, []byte(content), 0o644))

	execGit(t, workDir, "add", relPath)
	execGit(t, workDir, "commit", "-m", "test commit")
}

func commitSymlink(t *testing.T, workDir, relPath, target string) {
	t.Helper()

	fullPath := filepath.Join(workDir, relPath)
	require.NoError(t, os.MkdirAll(filepath.Dir(fullPath), 0o755))
	require.NoError(t, os.Symlink(target, fullPath))

	execGit(t, workDir, "add", relPath)
	execGit(t, workDir, "commit", "-m", "test symlink commit")
}

func currentMainSHA(bareDir string) string {
	cmd := exec.Command("git", "rev-parse", "refs/heads/main")
	cmd.Dir = bareDir

	out, err := cmd.CombinedOutput()
	if err != nil {
		return zeroSHA
	}

	return strings.TrimSpace(string(out))
}

func pushCommit(t *testing.T, workDir, bareDir string) (string, string) {
	t.Helper()

	oldSHA := currentMainSHA(bareDir)
	execGit(t, workDir, "push", bareDir, "HEAD:refs/heads/main")
	newSHA := execGit(t, workDir, "rev-parse", "HEAD")

	return oldSHA, newSHA
}

func updateLine(oldSHA, newSHA, ref string) string {
	return fmt.Sprintf("%s %s %s\n", oldSHA, newSHA, ref)
}

func TestValidate(t *testing.T) {
	t.Run("accepts valid whitelisted paths", func(t *testing.T) {
		bareDir := newBareRepo(t)
		workDir := newWorkTree(t)
		execGit(t, workDir, "remote", "add", "origin", bareDir)
		commitFile(t, workDir, "global/skills/foo/SKILL.md", "hello")

		oldSHA, newSHA := pushCommit(t, workDir, bareDir)

		err := githook.Validate(t.Context(), bareDir, strings.NewReader(updateLine(oldSHA, newSHA, "refs/heads/main")))
		require.NoError(t, err)
	})

	t.Run("rejects non-whitelisted path", func(t *testing.T) {
		bareDir := newBareRepo(t)
		workDir := newWorkTree(t)
		execGit(t, workDir, "remote", "add", "origin", bareDir)
		commitFile(t, workDir, "notes.txt", "hello")

		oldSHA, newSHA := pushCommit(t, workDir, bareDir)

		err := githook.Validate(t.Context(), bareDir, strings.NewReader(updateLine(oldSHA, newSHA, "refs/heads/main")))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "notes.txt")
	})

	t.Run("rejects wrong branch", func(t *testing.T) {
		bareDir := newBareRepo(t)

		stdin := updateLine(zeroSHA, strings.Repeat("a", 40), "refs/heads/feature")
		err := githook.Validate(t.Context(), bareDir, strings.NewReader(stdin))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "refs/heads/feature")
	})

	t.Run("rejects branch deletion", func(t *testing.T) {
		bareDir := newBareRepo(t)

		stdin := updateLine(strings.Repeat("a", 40), zeroSHA, "refs/heads/main")
		err := githook.Validate(t.Context(), bareDir, strings.NewReader(stdin))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "refs/heads/main")
	})

	t.Run("rejects symlink entry", func(t *testing.T) {
		bareDir := newBareRepo(t)
		workDir := newWorkTree(t)
		execGit(t, workDir, "remote", "add", "origin", bareDir)
		commitSymlink(t, workDir, "global/skills/link", "/etc/passwd")

		oldSHA, newSHA := pushCommit(t, workDir, bareDir)

		err := githook.Validate(t.Context(), bareDir, strings.NewReader(updateLine(oldSHA, newSHA, "refs/heads/main")))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "global/skills/link")
	})

	t.Run("rejects secret content", func(t *testing.T) {
		bareDir := newBareRepo(t)
		workDir := newWorkTree(t)
		execGit(t, workDir, "remote", "add", "origin", bareDir)
		commitFile(t, workDir, "global/skills/foo/creds.md", "api_key: abcdefgh12345678")

		oldSHA, newSHA := pushCommit(t, workDir, bareDir)

		err := githook.Validate(t.Context(), bareDir, strings.NewReader(updateLine(oldSHA, newSHA, "refs/heads/main")))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "global/skills/foo/creds.md")
	})

	t.Run("accepts docs that mention tokens in prose", func(t *testing.T) {
		bareDir := newBareRepo(t)
		workDir := newWorkTree(t)
		execGit(t, workDir, "remote", "add", "origin", bareDir)
		commitFile(t, workDir, "global/rules/forgejo.md", "Token: `~/.claude/skills/secrets/bin/get-secret forgejo` (probe chain).\nHeader: `Authorization: token <TOKEN>`.\n")

		oldSHA, newSHA := pushCommit(t, workDir, bareDir)

		err := githook.Validate(t.Context(), bareDir, strings.NewReader(updateLine(oldSHA, newSHA, "refs/heads/main")))
		require.NoError(t, err)
	})

	t.Run("multi-ref push where one ref is bad fails the whole push", func(t *testing.T) {
		bareDir := newBareRepo(t)
		workDir := newWorkTree(t)
		execGit(t, workDir, "remote", "add", "origin", bareDir)
		commitFile(t, workDir, "global/skills/foo/SKILL.md", "hello")

		oldSHA, newSHA := pushCommit(t, workDir, bareDir)

		var stdin strings.Builder
		stdin.WriteString(updateLine(oldSHA, newSHA, "refs/heads/main"))
		stdin.WriteString(updateLine(zeroSHA, strings.Repeat("b", 40), "refs/heads/other"))

		err := githook.Validate(t.Context(), bareDir, strings.NewReader(stdin.String()))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "refs/heads/other")
	})
}
