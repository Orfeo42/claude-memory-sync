package guardrails

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	hotBeginMarker = "<!-- HOT:BEGIN -->"
	hotEndMarker   = "<!-- HOT:END -->"
)

func ReplaceHotBlock(memoryDir string, digest []byte, maxBytes int) error {
	if len(digest) > maxBytes {
		return ErrDigestTooLarge
	}

	indexPath := filepath.Join(memoryDir, memoryIndexFilename)

	content, err := readIndexOrDefault(indexPath)
	if err != nil {
		return err
	}

	lines := splitLines(content)
	digestLines := splitLines(string(digest))

	updated := applyHotBlock(lines, digestLines)

	if err := os.WriteFile(indexPath, []byte(joinLines(updated)), 0o600); err != nil {
		return fmt.Errorf("write memory index: %w", err)
	}

	return nil
}

func readIndexOrDefault(indexPath string) (string, error) {
	content, err := os.ReadFile(indexPath)
	if os.IsNotExist(err) {
		return defaultIndexHeader, nil
	}
	if err != nil {
		return "", fmt.Errorf("read memory index: %w", err)
	}

	return string(content), nil
}

func applyHotBlock(lines, digestLines []string) []string {
	beginIdx, endIdx := findHotMarkers(lines)
	if beginIdx == -1 || endIdx == -1 {
		return insertHotBlockLines(lines, digestLines)
	}

	return replaceHotBlockLines(lines, beginIdx, endIdx, digestLines)
}

func findHotMarkers(lines []string) (int, int) {
	beginIdx, endIdx := -1, -1

	for i, line := range lines {
		if line == hotBeginMarker {
			beginIdx = i
		}
		if line == hotEndMarker {
			endIdx = i
		}
	}

	return beginIdx, endIdx
}

func replaceHotBlockLines(lines []string, beginIdx, endIdx int, digestLines []string) []string {
	result := make([]string, 0, len(lines)+len(digestLines))
	result = append(result, lines[:beginIdx+1]...)
	result = append(result, digestLines...)
	result = append(result, lines[endIdx:]...)

	return result
}

func insertHotBlockLines(lines, digestLines []string) []string {
	if len(lines) == 0 {
		lines = splitLines(defaultIndexHeader)
	}

	block := make([]string, 0, len(digestLines)+3)
	block = append(block, hotBeginMarker)
	block = append(block, digestLines...)
	block = append(block, hotEndMarker, "")

	result := make([]string, 0, len(lines)+len(block))
	result = append(result, lines[0])
	result = append(result, block...)
	result = append(result, lines[1:]...)

	return result
}

func splitLines(content string) []string {
	trimmed := strings.TrimSuffix(content, "\n")
	if trimmed == "" {
		return nil
	}

	return strings.Split(trimmed, "\n")
}

func joinLines(lines []string) string {
	return strings.Join(lines, "\n") + "\n"
}
