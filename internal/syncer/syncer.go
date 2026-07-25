package syncer

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"claude-memory-sync/internal/manifest"
)

type Agent struct {
	cfg    Config
	client HTTPClient
}

func New(cfg Config, client HTTPClient) *Agent {
	return &Agent{cfg: cfg, client: client}
}

func (a *Agent) RunCycle(ctx context.Context) error {
	local, localPaths, err := scanLocal(a.cfg.ClaudeDir, a.cfg.SlugPrefix)
	if err != nil {
		return fmt.Errorf("scan local: %w", err)
	}

	if err := a.upSync(ctx, local, localPaths); err != nil {
		return fmt.Errorf("up-sync: %w", err)
	}

	if err := a.downSync(ctx, local); err != nil {
		return fmt.Errorf("down-sync: %w", err)
	}

	return nil
}

func (a *Agent) clientBasePath() string {
	return filepath.Join(a.cfg.StateDir, clientBaseFile)
}

func (a *Agent) canonicalBasePath() string {
	return filepath.Join(a.cfg.StateDir, canonicalBaseFile)
}

func (a *Agent) upSync(ctx context.Context, local manifest.Manifest, localPaths map[string]string) error {
	base, err := loadManifest(a.clientBasePath())
	if err != nil {
		return err
	}

	diff := manifest.Diff(local, base, base)
	for path, change := range diff {
		switch change {
		case manifest.LocalOnlyChange:
			content, readErr := os.ReadFile(localPaths[path])
			if readErr != nil {
				return fmt.Errorf("read local file %s: %w", path, readErr)
			}
			if putErr := a.client.PutClientFile(ctx, path, content); putErr != nil {
				return fmt.Errorf("put client file %s: %w", path, putErr)
			}
			slog.InfoContext(ctx, "up-synced changed file", slog.String("path", path))
		case manifest.LocalDelete:
			if delErr := a.client.DeleteClientFile(ctx, path); delErr != nil {
				return fmt.Errorf("delete client file %s: %w", path, delErr)
			}
			slog.InfoContext(ctx, "up-synced deleted file", slog.String("path", path))
		default:
		}
	}

	return saveManifest(a.clientBasePath(), local)
}

func (a *Agent) downSync(ctx context.Context, local manifest.Manifest) error {
	canonicalCurrent, err := a.client.CanonicalTree(ctx)
	if err != nil {
		return fmt.Errorf("get canonical tree: %w", err)
	}

	canonicalBase, err := loadManifest(a.canonicalBasePath())
	if err != nil {
		return err
	}

	diff := manifest.Diff(local, canonicalBase, canonicalCurrent)
	for path, change := range diff {
		switch change {
		case manifest.RemoteOnlyChange:
			if applyErr := a.applyRemoteFile(ctx, path); applyErr != nil {
				return applyErr
			}
		case manifest.RemoteDelete:
			if delErr := a.applyRemoteDelete(path); delErr != nil {
				return delErr
			}
		case manifest.BothChanged:
			slog.WarnContext(ctx, "local wins, skipping canonical update", slog.String("path", path))
		default:
		}
	}

	return saveManifest(a.canonicalBasePath(), canonicalCurrent)
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

	content, err := a.client.GetCanonicalFile(ctx, path)
	if err != nil {
		return fmt.Errorf("get canonical file %s: %w", path, err)
	}

	if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
		return fmt.Errorf("create parent dir for %s: %w", target, err)
	}
	if err := os.WriteFile(target, content, 0o600); err != nil {
		return fmt.Errorf("write local file %s: %w", target, err)
	}

	slog.InfoContext(ctx, "down-synced changed file", slog.String("path", path))
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
