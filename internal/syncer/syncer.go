package syncer

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"claude-memory-sync/internal/domain"
	"claude-memory-sync/internal/manifest"
)

type Agent struct {
	cfg Config
}

func New(cfg Config) *Agent {
	return &Agent{cfg: cfg}
}

func (a *Agent) RunCycle(ctx context.Context) error {
	local, localPaths, err := scanLocal(a.cfg.ClaudeDir, a.cfg.SlugPrefix)
	if err != nil {
		return domain.Error(err, "scan local")
	}

	if err := a.upSync(ctx, local, localPaths); err != nil {
		return domain.Error(err, "up-sync")
	}

	if err := a.downSync(ctx, local); err != nil {
		return domain.Error(err, "down-sync")
	}

	return nil
}

func (a *Agent) canonicalBasePath() string {
	return filepath.Join(a.cfg.StateDir, canonicalBaseFile)
}

func (a *Agent) upSync(ctx context.Context, local manifest.Manifest, localPaths map[string]string) error {
	if err := ensureStagingRepo(ctx, a.cfg); err != nil {
		return err
	}
	if err := syncWorktree(stagingDir(a.cfg), local, localPaths); err != nil {
		return err
	}
	return commitAndPush(ctx, stagingDir(a.cfg))
}

func (a *Agent) downSync(ctx context.Context, local manifest.Manifest) error {
	if err := ensureCanonicalMirror(ctx, a.cfg); err != nil {
		return err
	}
	canonicalCurrent, err := canonicalManifest(a.cfg)
	if err != nil {
		return err
	}
	canonicalBase, err := loadManifest(a.canonicalBasePath())
	if err != nil {
		return domain.Error(err, "load canonical base")
	}

	diff := manifest.Diff(local, canonicalBase, canonicalCurrent)
	for path, change := range diff {
		if err := a.applyCanonicalChange(ctx, path, change); err != nil {
			return err
		}
	}

	if err := saveManifest(a.canonicalBasePath(), canonicalCurrent); err != nil {
		return domain.Error(err, "save canonical base")
	}
	return nil
}

func (a *Agent) applyCanonicalChange(ctx context.Context, path string, change manifest.ChangeType) error {
	switch change {
	case manifest.RemoteOnlyChange:
		return a.applyRemoteFile(ctx, path)
	case manifest.RemoteDelete:
		return a.applyRemoteDelete(path)
	case manifest.BothChanged:
		slog.WarnContext(ctx, "local wins, skipping canonical update", slog.String("path", path))
		return nil
	default:
		return nil
	}
}

func (a *Agent) applyRemoteFile(ctx context.Context, path string) error {
	target, err := localPath(path, a.cfg.ClaudeDir, a.cfg.SlugPrefix)
	if err != nil {
		slog.WarnContext(ctx, "skipping canonical file with unmappable path",
			slog.String("path", path),
			slog.String("error", err.Error()),
		)
		return nil
	}

	content, err := os.ReadFile(filepath.Join(mirrorDir(a.cfg), filepath.FromSlash(path)))
	if err != nil {
		return domain.Error(err, "read canonical mirror file", slog.String("path", path))
	}

	if err := writeUnderClaudeDir(a.cfg.ClaudeDir, target, content); err != nil {
		return err
	}

	slog.InfoContext(ctx, "down-synced changed file", slog.String("path", path))
	return nil
}

func writeUnderClaudeDir(claudeDir, target string, content []byte) error {
	rel, err := filepath.Rel(claudeDir, target)
	if err != nil {
		return domain.Error(err, "compute relative claude dir path", slog.String("path", target))
	}

	root, err := os.OpenRoot(claudeDir)
	if err != nil {
		return domain.Error(err, "open claude dir root", slog.String("path", claudeDir))
	}
	defer root.Close()

	if parent := filepath.Dir(rel); parent != "." {
		if err := root.MkdirAll(parent, 0o750); err != nil {
			return domain.Error(err, "create parent dir", slog.String("path", parent))
		}
	}
	if err := root.WriteFile(rel, content, 0o600); err != nil {
		return domain.Error(err, "write local file", slog.String("path", rel))
	}
	return nil
}

func (a *Agent) applyRemoteDelete(path string) error {
	target, err := localPath(path, a.cfg.ClaudeDir, a.cfg.SlugPrefix)
	if err != nil {
		slog.Warn("skipping canonical delete with unmappable path",
			slog.String("path", path),
			slog.String("error", err.Error()),
		)
		return nil
	}

	if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete local file %s: %w", target, err)
	}

	slog.Info("down-synced deleted file", slog.String("path", path))
	return nil
}
