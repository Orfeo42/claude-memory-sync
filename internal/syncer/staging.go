package syncer

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"claude-memory-sync/internal/domain"
	"claude-memory-sync/internal/gitutil"
	"claude-memory-sync/internal/manifest"
)

func stagingDir(cfg Config) string {
	return filepath.Join(cfg.StateDir, "staging")
}

func ensureStagingRepo(ctx context.Context, cfg Config) error {
	dir := stagingDir(cfg)
	gitDir := filepath.Join(dir, ".git")

	_, err := os.Stat(gitDir)
	switch {
	case os.IsNotExist(err):
		if initErr := initStagingRepo(ctx, cfg, dir); initErr != nil {
			return initErr
		}
	case err != nil:
		return domain.Error(err, "stat staging git dir", slog.String("path", gitDir))
	default:
		if setErr := gitutil.Run(ctx, dir, "remote", "set-url", "origin", clientRemoteURL(cfg)); setErr != nil {
			return domain.Error(setErr, "set staging remote url", slog.String("path", dir))
		}
	}

	if err := gitutil.Run(ctx, dir, "config", "http.extraHeader", "Authorization: Bearer "+cfg.Token); err != nil {
		return domain.Error(err, "set staging auth header", slog.String("path", dir))
	}

	return nil
}

func initStagingRepo(ctx context.Context, cfg Config, dir string) error {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return domain.Error(err, "create staging dir", slog.String("path", dir))
	}
	if err := gitutil.Run(ctx, dir, "init", "-b", "main"); err != nil {
		return domain.Error(err, "init staging repo", slog.String("path", dir))
	}
	if err := gitutil.Run(ctx, dir, "config", "user.name", "memory-agent"); err != nil {
		return domain.Error(err, "set staging user name", slog.String("path", dir))
	}
	if err := gitutil.Run(ctx, dir, "config", "user.email", cfg.ClientID+"@memory-agent.local"); err != nil {
		return domain.Error(err, "set staging user email", slog.String("path", dir))
	}
	if err := gitutil.Run(ctx, dir, "remote", "add", "origin", clientRemoteURL(cfg)); err != nil {
		return domain.Error(err, "add staging remote", slog.String("path", dir))
	}
	return nil
}

func syncWorktree(dir string, local manifest.Manifest, localPaths map[string]string) error {
	if err := removeStaleStagingFiles(dir, local); err != nil {
		return err
	}
	return writeStagingFiles(dir, local, localPaths)
}

func removeStaleStagingFiles(dir string, local manifest.Manifest) error {
	localMap := local.Map()

	root, err := os.OpenRoot(dir)
	if err != nil {
		return domain.Error(err, "open staging root", slog.String("path", dir))
	}
	defer root.Close()

	walkErr := fs.WalkDir(root.FS(), ".", func(relPath string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if relPath == "." {
			return nil
		}
		if d.IsDir() && d.Name() == ".git" && filepath.Dir(relPath) == "." {
			return fs.SkipDir
		}
		if d.IsDir() {
			return nil
		}

		if _, ok := localMap[relPath]; ok {
			return nil
		}
		return root.Remove(filepath.FromSlash(relPath))
	})
	if walkErr != nil {
		return domain.Error(walkErr, "remove stale staging files", slog.String("path", dir))
	}
	return nil
}

func writeStagingFiles(dir string, local manifest.Manifest, localPaths map[string]string) error {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return domain.Error(err, "open staging root", slog.String("path", dir))
	}
	defer root.Close()

	for _, entry := range local {
		src := localPaths[entry.Path]
		relName := filepath.FromSlash(entry.Path)

		content, readErr := os.ReadFile(src)
		if readErr != nil {
			return domain.Error(readErr, "read local source file", slog.String("path", src))
		}

		if parent := filepath.Dir(relName); parent != "." {
			if mkErr := root.MkdirAll(parent, 0o750); mkErr != nil {
				return domain.Error(mkErr, "create staging parent dir", slog.String("path", parent))
			}
		}
		if writeErr := root.WriteFile(relName, content, 0o600); writeErr != nil {
			return domain.Error(writeErr, "write staging file", slog.String("path", relName))
		}
	}
	return nil
}

func commitAndPush(ctx context.Context, dir string) error {
	if err := gitutil.Run(ctx, dir, "add", "-A"); err != nil {
		return domain.Error(err, "stage staging changes", slog.String("path", dir))
	}

	hasStaged, err := gitutil.HasStagedChanges(ctx, dir)
	if err != nil {
		return domain.Error(err, "check staged changes", slog.String("path", dir))
	}
	if !hasStaged {
		return nil
	}

	out, err := gitutil.RunOutput(ctx, dir, "diff", "--cached", "--name-only")
	if err != nil {
		return domain.Error(err, "list staged changes", slog.String("path", dir))
	}
	changed := countChangedPaths(out)

	if err := gitutil.Run(ctx, dir, "commit", "-m", fmt.Sprintf("sync: %d paths changed", changed)); err != nil {
		return domain.Error(err, "commit staging changes", slog.String("path", dir))
	}
	if err := gitutil.Run(ctx, dir, "push", "origin", "main"); err != nil {
		return domain.Error(err, "push staging changes", slog.String("path", dir))
	}

	return nil
}

func countChangedPaths(out string) int {
	if out == "" {
		return 0
	}

	lines := strings.Split(out, "\n")
	count := 0
	for _, line := range lines {
		if line != "" {
			count++
		}
	}
	return count
}
