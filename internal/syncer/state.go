package syncer

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Orfeo42/claude-memory-sync/internal/manifest"
)

const (
	clientBaseFile    = "client.json"
	canonicalBaseFile = "canonical.json"
)

func loadManifest(path string) (manifest.Manifest, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return manifest.Manifest{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read state %s: %w", path, err)
	}

	var m manifest.Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("unmarshal state %s: %w", path, err)
	}
	return m, nil
}

func saveManifest(path string, m manifest.Manifest) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create state dir: %w", err)
	}

	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write state %s: %w", path, err)
	}
	return nil
}
