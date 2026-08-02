package guardrails

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	memoryIndexFilename = "MEMORY.md"
	defaultIndexHeader  = "# Memory index\n\n"
	indexBulletPrefix   = "- ["
)

func RebuildIndex(memoryDir string) error {
	indexPath := filepath.Join(memoryDir, memoryIndexFilename)

	header, err := buildIndexHeader(indexPath)
	if err != nil {
		return err
	}

	names, err := listEntryNames(memoryDir)
	if err != nil {
		return err
	}

	var body strings.Builder
	body.WriteString(header)

	for _, name := range names {
		line, lineErr := buildIndexLine(memoryDir, name)
		if lineErr != nil {
			return lineErr
		}

		body.WriteString(line)
	}

	if err := os.WriteFile(indexPath, []byte(body.String()), 0o600); err != nil {
		return fmt.Errorf("write memory index: %w", err)
	}

	return nil
}

func buildIndexHeader(indexPath string) (string, error) {
	content, err := os.ReadFile(indexPath)
	if os.IsNotExist(err) {
		return defaultIndexHeader, nil
	}
	if err != nil {
		return "", fmt.Errorf("read memory index: %w", err)
	}

	if len(content) == 0 {
		return defaultIndexHeader, nil
	}

	lines := strings.Split(string(content), "\n")

	var header strings.Builder
	for _, line := range lines {
		if strings.HasPrefix(line, indexBulletPrefix) {
			return header.String(), nil
		}

		header.WriteString(line)
		header.WriteString("\n")
	}

	return string(content), nil
}

func listEntryNames(memoryDir string) ([]string, error) {
	entries, err := os.ReadDir(memoryDir)
	if err != nil {
		return nil, fmt.Errorf("read memory dir: %w", err)
	}

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if name == memoryIndexFilename || !strings.HasSuffix(name, ".md") {
			continue
		}

		names = append(names, name)
	}

	sort.Strings(names)

	return names, nil
}

func buildIndexLine(memoryDir, name string) (string, error) {
	content, err := os.ReadFile(filepath.Join(memoryDir, name))
	if err != nil {
		return "", fmt.Errorf("read entry %q: %w", name, err)
	}

	fmName := extractFrontmatterField(content, "name")
	fmDescription := extractFrontmatterField(content, "description")

	return fmt.Sprintf("- [%s](%s) — %s\n", fmName, name, stripQuotes(fmDescription)), nil
}

func extractFrontmatterField(content []byte, field string) string {
	lines := strings.Split(string(content), "\n")
	if len(lines) == 0 || lines[0] != frontmatterDelimiter {
		return ""
	}

	prefix := field + ":"
	offset := len(field) + 2

	for _, line := range lines[1:] {
		if line == frontmatterDelimiter {
			return ""
		}

		if !strings.HasPrefix(line, prefix) {
			continue
		}

		if len(line) < offset {
			return ""
		}

		return line[offset:]
	}

	return ""
}
