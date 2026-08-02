package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func initBareRepo(t *testing.T, path string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
	runGit(t, "", nil, "init", "--bare", "--initial-branch=main", path)
}

func runGit(t *testing.T, dir string, env []string, args ...string) string {
	t.Helper()

	cmd := exec.Command("git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	if env != nil {
		cmd.Env = append(os.Environ(), env...)
	}

	out, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "git %v: %s", args, out)
	return string(out)
}

func dateEnv(when time.Time) []string {
	formatted := when.Format(time.RFC3339)
	return []string{"GIT_AUTHOR_DATE=" + formatted, "GIT_COMMITTER_DATE=" + formatted}
}

func commitFiles(t *testing.T, bareRepoPath string, files map[string]string, message string, when time.Time) string {
	t.Helper()

	scratch := t.TempDir()
	runGit(t, "", nil, "clone", "file://"+bareRepoPath, scratch)

	for relPath, content := range files {
		full := filepath.Join(scratch, filepath.FromSlash(relPath))
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o750))
		require.NoError(t, os.WriteFile(full, []byte(content), 0o600))
	}

	runGit(t, scratch, nil, "config", "user.name", "test-seed")
	runGit(t, scratch, nil, "config", "user.email", "test-seed@test.local")
	runGit(t, scratch, nil, "add", "-A")
	runGit(t, scratch, dateEnv(when), "commit", "-m", message)
	runGit(t, scratch, nil, "push", "origin", "main")

	return strings.TrimSpace(runGit(t, scratch, nil, "rev-parse", "HEAD"))
}

func removeFiles(t *testing.T, bareRepoPath string, paths []string, message string, when time.Time) string {
	t.Helper()

	scratch := t.TempDir()
	runGit(t, "", nil, "clone", "file://"+bareRepoPath, scratch)

	runGit(t, scratch, nil, "config", "user.name", "test-seed")
	runGit(t, scratch, nil, "config", "user.email", "test-seed@test.local")
	runGit(t, scratch, nil, append([]string{"rm"}, paths...)...)
	runGit(t, scratch, dateEnv(when), "commit", "-m", message)
	runGit(t, scratch, nil, "push", "origin", "main")

	return strings.TrimSpace(runGit(t, scratch, nil, "rev-parse", "HEAD"))
}
