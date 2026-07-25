package manifest

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

type Entry struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

type Manifest []Entry

func (m Manifest) Map() map[string]Entry {
	result := make(map[string]Entry, len(m))
	for _, e := range m {
		result[e.Path] = e
	}
	return result
}

type WhitelistFunc func(relPath string, d fs.DirEntry) bool

func Build(root string, whitelist WhitelistFunc) (Manifest, error) {
	result := Manifest{}

	_, err := os.Stat(root)
	if os.IsNotExist(err) {
		return result, nil
	}
	if err != nil {
		return nil, fmt.Errorf("stat root: %w", err)
	}

	walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == root {
			return nil
		}

		relPath, relErr := relSlashPath(root, path)
		if relErr != nil {
			return relErr
		}

		if d.IsDir() {
			return walkDir(relPath, d, whitelist)
		}

		if skipFile(d, relPath, whitelist) {
			return nil
		}

		entry, entryErr := buildEntry(path, relPath, d)
		if entryErr != nil {
			return entryErr
		}
		result = append(result, entry)
		return nil
	})
	if walkErr != nil {
		return nil, fmt.Errorf("walk %s: %w", root, walkErr)
	}

	return result, nil
}

func relSlashPath(root, path string) (string, error) {
	relPath, err := filepath.Rel(root, path)
	if err != nil {
		return "", fmt.Errorf("relative path: %w", err)
	}
	return filepath.ToSlash(relPath), nil
}

func walkDir(relPath string, d fs.DirEntry, whitelist WhitelistFunc) error {
	if whitelist(relPath, d) {
		return nil
	}
	return filepath.SkipDir
}

func skipFile(d fs.DirEntry, relPath string, whitelist WhitelistFunc) bool {
	if d.Type()&fs.ModeSymlink != 0 {
		return true
	}
	return !whitelist(relPath, d)
}

func buildEntry(path, relPath string, d fs.DirEntry) (Entry, error) {
	info, err := d.Info()
	if err != nil {
		return Entry{}, fmt.Errorf("file info: %w", err)
	}

	sum, err := HashFile(path)
	if err != nil {
		return Entry{}, fmt.Errorf("hash file: %w", err)
	}

	return Entry{
		Path:   relPath,
		SHA256: sum,
		Size:   info.Size(),
	}, nil
}

func HashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open file: %w", err)
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("hash content: %w", err)
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}
