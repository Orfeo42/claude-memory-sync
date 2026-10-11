package pipeline

import (
	pathpkg "path"
	"strings"

	"claude-memory-sync/internal/domain"
)

type Kind string

const (
	KindSkill        Kind = "skill"
	KindRule         Kind = "rule"
	KindAgent        Kind = "agent"
	KindGlobal       Kind = "global"
	KindProjectEntry Kind = "project-entry"
	KindBug          Kind = "bug"
	KindOneTimeTask  Kind = "one-time-task"
	KindCodeChange   Kind = "code-change"
)

const (
	bugPrefix  = "bug-"
	taskPrefix = "task-"
	todoPrefix = "todo-"
)

func ParseKind(s string) (Kind, error) {
	switch kind := Kind(s); kind {
	case KindSkill, KindRule, KindAgent, KindGlobal, KindProjectEntry, KindBug, KindOneTimeTask, KindCodeChange:
		return kind, nil
	default:
		return "", domain.Error(domain.ErrValidation, "unknown proposal kind")
	}
}

func KindOf(target string) Kind {
	switch {
	case strings.HasPrefix(target, skillsPrefix):
		return KindSkill
	case strings.HasPrefix(target, rulesPrefix):
		return KindRule
	case strings.HasPrefix(target, agentsPrefix):
		return KindAgent
	case strings.HasPrefix(target, globalPrefix):
		return KindGlobal
	}

	name := pathpkg.Base(target)
	switch {
	case strings.HasPrefix(name, bugPrefix):
		return KindBug
	case strings.HasPrefix(name, taskPrefix):
		return KindOneTimeTask
	case strings.HasPrefix(name, todoPrefix):
		return KindCodeChange
	default:
		return KindProjectEntry
	}
}

func GroupOf(target string) string {
	parts := strings.Split(target, "/")
	if strings.HasPrefix(target, skillsPrefix) && len(parts) > 2 {
		return parts[2]
	}
	return pathpkg.Base(target)
}

func PathspecOf(target string) string {
	parts := strings.Split(target, "/")
	if parts[0] == projectsDir && len(parts) > 1 {
		return projectsDir + "/" + parts[1]
	}
	return strings.TrimSuffix(globalPrefix, "/")
}
