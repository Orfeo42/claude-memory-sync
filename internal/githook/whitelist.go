package githook

import (
	"fmt"
	"log/slog"
	"regexp"
	"strings"

	"claude-memory-sync/internal/domain"
)

var whitelistPattern = regexp.MustCompile(strings.Join([]string{
	`^global/CLAUDE\.md$`,
	`^global/rules/[^/]+\.md$`,
	`^global/skills/.+$`,
	`^global/agents/.+$`,
	`^projects/[^/]+/memory/.+$`,
}, "|"))

func checkWhitelist(path string) error {
	if whitelistPattern.MatchString(path) {
		return nil
	}

	return domain.Error(errPathNotAllowed, fmt.Sprintf("path %q is not in the sync whitelist", path), slog.String("path", path))
}
