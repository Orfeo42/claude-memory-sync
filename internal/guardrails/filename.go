package guardrails

import (
	"fmt"
	"regexp"
	"strings"
)

var filenamePattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*\.md$`)

func NormalizeFilename(name string) (string, error) {
	normalized := normalizeFilenameCase(name)

	if normalized == "memory.md" {
		return "", ErrIndexTarget
	}

	if !filenamePattern.MatchString(normalized) {
		return "", fmt.Errorf("%q: %w", name, ErrInvalidFilename)
	}

	return normalized, nil
}

func normalizeFilenameCase(name string) string {
	lower := strings.ToLower(name)
	return strings.ReplaceAll(lower, "_", "-")
}
