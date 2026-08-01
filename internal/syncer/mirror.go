package syncer

import (
	"context"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"

	"claude-memory-sync/internal/domain"
	"claude-memory-sync/internal/gitutil"
	"claude-memory-sync/internal/manifest"
)

func mirrorDir(cfg Config) string {
	return filepath.Join(cfg.StateDir, "canonical-mirror")
}

func ensureCanonicalMirror(ctx context.Context, cfg Config) error {
	dir := mirrorDir(cfg)
	gitDir := filepath.Join(dir, ".git")

	_, err := os.Stat(gitDir)
	switch {
	case os.IsNotExist(err):
		if cloneErr := cloneCanonicalMirror(ctx, cfg, dir); cloneErr != nil {
			return cloneErr
		}
	case err != nil:
		return domain.Error(err, "stat canonical mirror git dir", slog.String("path", gitDir))
	}

	if err := gitutil.Run(ctx, dir, "config", "http.extraHeader", "Authorization: Bearer "+cfg.Token); err != nil {
		return domain.Error(err, "set canonical mirror auth header", slog.String("path", dir))
	}

	if err := gitutil.Run(ctx, dir, "fetch", "origin"); err != nil {
		return domain.Error(err, "fetch canonical mirror", slog.String("path", dir))
	}

	if !canonicalMainRefExists(ctx, dir) {
		slog.WarnContext(ctx, "canonical repo has no commits yet", slog.String("path", dir))
		return nil
	}

	if err := gitutil.Run(ctx, dir, "reset", "--hard", "origin/main"); err != nil {
		return domain.Error(err, "reset canonical mirror", slog.String("path", dir))
	}

	return nil
}

func canonicalMainRefExists(ctx context.Context, dir string) bool {
	_, err := gitutil.RunOutput(ctx, dir, "rev-parse", "--verify", "--quiet", "origin/main")
	return err == nil
}

func cloneCanonicalMirror(ctx context.Context, cfg Config, dir string) error {
	parent := filepath.Dir(dir)
	if err := os.MkdirAll(parent, 0o750); err != nil {
		return domain.Error(err, "create canonical mirror parent dir", slog.String("path", parent))
	}
	if err := gitutil.Run(ctx, parent, "clone", "-c", "http.extraHeader=Authorization: Bearer "+cfg.Token, canonicalRemoteURL(cfg), filepath.Base(dir)); err != nil {
		return domain.Error(err, "clone canonical mirror", slog.String("path", dir))
	}
	return nil
}

func excludeGitDir(relPath string, d fs.DirEntry) bool {
	if d.IsDir() && relPath == ".git" {
		return false
	}
	return true
}

func canonicalManifest(cfg Config) (manifest.Manifest, error) {
	dir := mirrorDir(cfg)
	m, err := manifest.Build(dir, excludeGitDir)
	if err != nil {
		return nil, domain.Error(err, "build canonical manifest", slog.String("path", dir))
	}
	return m, nil
}
