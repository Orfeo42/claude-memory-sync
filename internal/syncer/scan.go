package syncer

import (
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"

	"claude-memory-sync/internal/manifest"
)

func scanLocal(claudeDir, slugPrefix string) (manifest.Manifest, map[string]string, error) {
	result := manifest.Manifest{}
	paths := map[string]string{}

	claudeEntry, claudePath, err := scanClaudeMD(claudeDir)
	if err != nil {
		return nil, nil, err
	}
	if claudeEntry != nil {
		result = append(result, *claudeEntry)
		paths[claudeEntry.Path] = claudePath
	}

	ruleEntries, rulePaths, err := scanRules(claudeDir)
	if err != nil {
		return nil, nil, err
	}
	result = append(result, ruleEntries...)
	for path, fsPath := range rulePaths {
		paths[path] = fsPath
	}

	projectEntries, projectPaths, err := scanProjects(claudeDir, slugPrefix)
	if err != nil {
		return nil, nil, err
	}
	result = append(result, projectEntries...)
	for path, fsPath := range projectPaths {
		paths[path] = fsPath
	}

	return result, paths, nil
}

func scanClaudeMD(claudeDir string) (*manifest.Entry, string, error) {
	path := filepath.Join(claudeDir, "CLAUDE.md")

	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil, "", nil
	}
	if err != nil {
		return nil, "", fmt.Errorf("lstat %s: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		slog.Warn("skipping symlinked CLAUDE.md, machine not yet migrated", slog.String("path", path))
		return nil, "", nil
	}

	sum, err := manifest.HashFile(path)
	if err != nil {
		return nil, "", fmt.Errorf("hash %s: %w", path, err)
	}

	return &manifest.Entry{Path: "global/CLAUDE.md", SHA256: sum, Size: info.Size()}, path, nil
}

func scanRules(claudeDir string) (manifest.Manifest, map[string]string, error) {
	rulesDir := filepath.Join(claudeDir, "rules")

	dirInfo, err := os.Lstat(rulesDir)
	if os.IsNotExist(err) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("lstat %s: %w", rulesDir, err)
	}
	if dirInfo.Mode()&os.ModeSymlink != 0 {
		slog.Warn("skipping symlinked rules dir, machine not yet migrated", slog.String("path", rulesDir))
		return nil, nil, nil
	}

	entries, err := os.ReadDir(rulesDir)
	if err != nil {
		return nil, nil, fmt.Errorf("read dir %s: %w", rulesDir, err)
	}

	result := manifest.Manifest{}
	paths := map[string]string{}
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".md" {
			continue
		}

		filePath := filepath.Join(rulesDir, e.Name())
		fileInfo, lErr := os.Lstat(filePath)
		if lErr != nil {
			return nil, nil, fmt.Errorf("lstat %s: %w", filePath, lErr)
		}
		if fileInfo.Mode()&os.ModeSymlink != 0 {
			slog.Warn("skipping symlinked rule file, machine not yet migrated", slog.String("path", filePath))
			continue
		}

		sum, hErr := manifest.HashFile(filePath)
		if hErr != nil {
			return nil, nil, fmt.Errorf("hash %s: %w", filePath, hErr)
		}

		nsPath := "global/rules/" + e.Name()
		result = append(result, manifest.Entry{Path: nsPath, SHA256: sum, Size: fileInfo.Size()})
		paths[nsPath] = filePath
	}

	return result, paths, nil
}

func scanProjects(claudeDir, slugPrefix string) (manifest.Manifest, map[string]string, error) {
	projectsDir := filepath.Join(claudeDir, "projects")

	entries, err := os.ReadDir(projectsDir)
	if os.IsNotExist(err) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("read dir %s: %w", projectsDir, err)
	}

	result := manifest.Manifest{}
	paths := map[string]string{}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}

		slug := e.Name()
		key, ok := canonicalKey(slug, slugPrefix)
		if !ok {
			slog.Warn("skipping project slug not matching prefix",
				slog.String("slug", slug),
				slog.String("prefix", slugPrefix),
			)
			continue
		}

		memoryDir := filepath.Join(projectsDir, slug, "memory")
		memoryManifest, buildErr := manifest.Build(memoryDir, func(string, fs.DirEntry) bool { return true })
		if buildErr != nil {
			return nil, nil, fmt.Errorf("scan project memory %s: %w", slug, buildErr)
		}

		for _, entry := range memoryManifest {
			nsPath := "projects/" + key + "/memory/" + entry.Path
			result = append(result, manifest.Entry{Path: nsPath, SHA256: entry.SHA256, Size: entry.Size})
			paths[nsPath] = filepath.Join(memoryDir, filepath.FromSlash(entry.Path))
		}
	}

	return result, paths, nil
}
