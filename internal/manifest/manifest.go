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

		relPath, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return fmt.Errorf("relative path: %w", relErr)
		}
		relPath = filepath.ToSlash(relPath)

		if d.IsDir() {
			if !whitelist(relPath, d) {
				return filepath.SkipDir
			}
			return nil
		}

		if d.Type()&fs.ModeSymlink != 0 {
			return nil
		}

		if !whitelist(relPath, d) {
			return nil
		}

		info, infoErr := d.Info()
		if infoErr != nil {
			return fmt.Errorf("file info: %w", infoErr)
		}

		sum, hashErr := HashFile(path)
		if hashErr != nil {
			return fmt.Errorf("hash file: %w", hashErr)
		}

		result = append(result, Entry{
			Path:   relPath,
			SHA256: sum,
			Size:   info.Size(),
		})
		return nil
	})
	if walkErr != nil {
		return nil, fmt.Errorf("walk %s: %w", root, walkErr)
	}

	return result, nil
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
