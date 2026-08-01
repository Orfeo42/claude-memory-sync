package gitserver

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"claude-memory-sync/internal/clientid"
	"claude-memory-sync/internal/domain"
	"claude-memory-sync/internal/gitutil"
)

const (
	canonicalRepoName  = "canonical.git"
	clientsRepoPrefix  = "clients/"
	repoSuffix         = ".git"
	repoDirMode        = 0o755
	hookFileMode       = 0o755
	preReceiveHookName = "pre-receive"
)

func repoDir(reposRoot, repo string) string {
	return filepath.Join(reposRoot, repo)
}

func isCanonicalRepo(repo string) bool {
	return repo == canonicalRepoName
}

func parseClientRepo(repo string) (string, bool) {
	if !strings.HasPrefix(repo, clientsRepoPrefix) || !strings.HasSuffix(repo, repoSuffix) {
		return "", false
	}

	id := strings.TrimSuffix(strings.TrimPrefix(repo, clientsRepoPrefix), repoSuffix)
	if !clientid.Valid(id) {
		return "", false
	}

	return id, true
}

func validRepoName(repo string) bool {
	if isCanonicalRepo(repo) {
		return true
	}

	_, ok := parseClientRepo(repo)
	return ok
}

func repoExists(dir string) (bool, error) {
	_, err := os.Stat(filepath.Join(dir, "HEAD"))
	if err == nil {
		return true, nil
	}

	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}

	return false, fmt.Errorf("stat repository head: %w", err)
}

func initBareRepo(ctx context.Context, dir string) error {
	if err := os.MkdirAll(dir, repoDirMode); err != nil {
		return fmt.Errorf("mkdir repo dir: %w", err)
	}

	if err := gitutil.Run(ctx, dir, "init", "--bare", "--initial-branch=main"); err != nil {
		return fmt.Errorf("git init --bare: %w", err)
	}

	return nil
}

func ensureClientRepo(ctx context.Context, dir, hookCommand string) error {
	exists, err := repoExists(dir)
	if err != nil {
		return err
	}

	if exists {
		return nil
	}

	if err := initBareRepo(ctx, dir); err != nil {
		return err
	}

	return installPreReceiveHook(dir, hookCommand)
}

func installPreReceiveHook(dir, hookCommand string) error {
	if hookCommand == "" {
		return errHookCommandUnresolved
	}

	hookPath := filepath.Join(dir, "hooks", preReceiveHookName)
	content := fmt.Sprintf("#!/bin/sh\nexec %s -githook-pre-receive\n", hookCommand)

	if err := os.WriteFile(hookPath, []byte(content), hookFileMode); err != nil {
		return fmt.Errorf("write pre-receive hook: %w", err)
	}

	return nil
}

func EnsureCanonical(ctx context.Context, reposRoot string) error {
	dir := repoDir(reposRoot, canonicalRepoName)

	exists, err := repoExists(dir)
	if err != nil {
		return domain.Error(err, "failed to check canonical repository state")
	}

	if exists {
		return nil
	}

	if err := initBareRepo(ctx, dir); err != nil {
		return domain.Error(err, "failed to initialize canonical repository")
	}

	return nil
}

func (h *handler) resolveRepoDir(ctx context.Context, repo, service string) (string, error) {
	dir := repoDir(h.reposRoot, repo)

	if isCanonicalRepo(repo) {
		return dir, nil
	}

	clientID, ok := parseClientRepo(repo)
	if !ok {
		return "", domain.Error(errRepoNotFound, "unknown repository", errAttrs(repo)...)
	}

	exists, err := repoExists(dir)
	if err != nil {
		return "", domain.Error(err, "failed to stat client repository", errAttrsClient(clientID)...)
	}

	if exists {
		return dir, nil
	}

	if service != serviceReceivePack {
		return "", domain.Error(errRepoNotFound, "client repository not found", errAttrsClient(clientID)...)
	}

	if err := ensureClientRepo(ctx, dir, h.hookCommand); err != nil {
		return "", domain.Error(err, "failed to auto-create client repository", errAttrsClient(clientID)...)
	}

	return dir, nil
}
