package syncer

import (
	"fmt"
	"path/filepath"
	"strings"
)

const globalClaudeMDPath = "global/CLAUDE.md"

func localPath(nsPath, claudeDir, slugPrefix string) (string, error) {
	switch {
	case nsPath == globalClaudeMDPath:
		return filepath.Join(claudeDir, "CLAUDE.md"), nil

	case strings.HasPrefix(nsPath, "global/rules/"):
		rest := strings.TrimPrefix(nsPath, "global/rules/")
		if rest == "" {
			return "", fmt.Errorf("%w: %s", errInvalidNamespacePath, nsPath)
		}
		return filepath.Join(claudeDir, "rules", filepath.FromSlash(rest)), nil

	case strings.HasPrefix(nsPath, "projects/"):
		rest := strings.TrimPrefix(nsPath, "projects/")
		parts := strings.SplitN(rest, "/memory/", 2)
		if len(parts) != 2 || parts[1] == "" {
			return "", fmt.Errorf("%w: %s", errInvalidNamespacePath, nsPath)
		}

		slug, ok := reverseSlug(parts[0], slugPrefix)
		if !ok {
			return "", fmt.Errorf("%w: %s", errUnknownCanonicalKey, parts[0])
		}
		return filepath.Join(claudeDir, "projects", slug, "memory", filepath.FromSlash(parts[1])), nil

	default:
		return "", fmt.Errorf("%w: %s", errInvalidNamespacePath, nsPath)
	}
}
