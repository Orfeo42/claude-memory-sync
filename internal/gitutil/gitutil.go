package gitutil

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"claude-memory-sync/internal/domain"
)

func Run(ctx context.Context, dir string, args ...string) error {
	_, err := RunOutput(ctx, dir, args...)
	return err
}

func RunOutput(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %v: %w: %s", args, err, stderr.String())
	}

	return strings.TrimRight(stdout.String(), "\n"), nil
}

func HasStagedChanges(ctx context.Context, dir string) (bool, error) {
	cmd := exec.CommandContext(ctx, "git", "-C", dir, "diff", "--cached", "--quiet")
	err := cmd.Run()
	if err == nil {
		return false, nil
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
		return true, nil
	}

	return false, fmt.Errorf("git diff --cached: %w", err)
}

func IsAncestor(ctx context.Context, dir, ancestor, descendant string) (bool, error) {
	cmd := exec.CommandContext(ctx, "git", "-C", dir, "merge-base", "--is-ancestor", ancestor, descendant)
	err := cmd.Run()
	if err == nil {
		return true, nil
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
		return false, nil
	}

	return false, fmt.Errorf("git merge-base --is-ancestor: %w", err)
}

func WithFileLock(path string, fn func() error) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return domain.Error(err, "create lock dir failed", slog.String("path", path))
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return domain.Error(err, "open lock file failed")
	}
	defer func() {
		_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
		_ = file.Close()
	}()

	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX); err != nil {
		return domain.Error(err, "acquire file lock failed")
	}

	return fn()
}
