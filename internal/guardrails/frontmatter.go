package guardrails

import "strings"

const frontmatterDelimiter = "---"

func ValidateFrontmatter(content []byte) error {
	lines := strings.Split(string(content), "\n")

	if len(lines) == 0 || lines[0] != frontmatterDelimiter {
		return ErrNoFrontmatter
	}

	closeIdx := indexOfExact(lines[1:], frontmatterDelimiter)
	if closeIdx == -1 {
		return ErrNoFrontmatter
	}

	block := lines[1 : closeIdx+1]

	if extractTopLevelValue(block, "name") == "" {
		return ErrMissingName
	}

	if extractTopLevelValue(block, "description") == "" {
		return ErrMissingDescription
	}

	return validateMetadataType(block)
}

func validateMetadataType(block []string) error {
	metaIdx := indexOfPrefix(block, "metadata:")
	if metaIdx == -1 {
		return ErrMissingMetadataType
	}

	for _, line := range block[metaIdx+1:] {
		if !isIndented(line) {
			continue
		}

		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "type:") {
			continue
		}

		value := stripQuotes(strings.TrimSpace(strings.TrimPrefix(trimmed, "type:")))
		if value == "" {
			return ErrMissingMetadataType
		}

		return nil
	}

	return ErrMissingMetadataType
}

func extractTopLevelValue(block []string, field string) string {
	prefix := field + ":"

	for _, line := range block {
		if !strings.HasPrefix(line, prefix) {
			continue
		}

		value := strings.TrimSpace(strings.TrimPrefix(line, prefix))
		return stripQuotes(value)
	}

	return ""
}

func stripQuotes(value string) string {
	if len(value) == 0 {
		return value
	}

	switch value[0] {
	case '"':
		return strings.TrimSuffix(value[1:], `"`)
	case '\'':
		return strings.TrimSuffix(value[1:], `'`)
	default:
		return value
	}
}

func isIndented(line string) bool {
	return len(line) > 0 && (line[0] == ' ' || line[0] == '\t')
}

func indexOfPrefix(lines []string, prefix string) int {
	for i, line := range lines {
		if strings.HasPrefix(line, prefix) {
			return i
		}
	}

	return -1
}

func indexOfExact(lines []string, value string) int {
	for i, line := range lines {
		if line == value {
			return i
		}
	}

	return -1
}
