package pipeline

import (
	pathpkg "path"
	"strings"

	"claude-memory-sync/internal/domain"
)

const (
	globalPrefix    = "global/"
	rulesPrefix     = "global/rules/"
	skillsPrefix    = "global/skills/"
	agentsPrefix    = "global/agents/"
	claudeMDPath    = "global/CLAUDE.md"
	projectsDir     = "projects"
	memoryDir       = "memory"
	memoryIndexFile = "MEMORY.md"
	markdownSuffix  = ".md"
)

func ValidateTarget(p string) error {
	if p == "" {
		return domain.Error(domain.ErrValidation, "proposal path is empty")
	}
	if strings.Contains(p, "..") || pathpkg.IsAbs(p) || pathpkg.Clean(p) != p {
		return domain.Error(domain.ErrValidation, "proposal path is unsafe")
	}

	switch {
	case p == claudeMDPath:
		return nil
	case strings.HasPrefix(p, rulesPrefix):
		return requireMarkdown(p)
	case strings.HasPrefix(p, skillsPrefix), strings.HasPrefix(p, agentsPrefix):
		return nil
	case isProjectMemoryTarget(p):
		return nil
	default:
		return domain.Error(domain.ErrValidation, "proposal path is not in an allowed location")
	}
}

func isProjectMemoryTarget(p string) bool {
	parts := strings.Split(p, "/")
	if len(parts) != 4 || parts[0] != projectsDir || parts[2] != memoryDir {
		return false
	}
	name := parts[3]
	return parts[1] != "" && name != memoryIndexFile && strings.HasSuffix(name, markdownSuffix)
}

func requireMarkdown(p string) error {
	if !strings.HasSuffix(p, markdownSuffix) {
		return domain.Error(domain.ErrValidation, "proposal path requires .md extension")
	}
	return nil
}
