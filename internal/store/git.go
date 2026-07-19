package store

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
)

func runGit(ctx context.Context, root string, args ...string) error {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", root}, args...)...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git %v: %w: %s", args, err, stderr.String())
	}

	return nil
}

func gitHasStagedChanges(ctx context.Context, root string) (bool, error) {
	cmd := exec.CommandContext(ctx, "git", "-C", root, "diff", "--cached", "--quiet")
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
