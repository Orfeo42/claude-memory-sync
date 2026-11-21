package settings

import (
	"log/slog"
	"regexp"
	"strings"

	"claude-memory-sync/internal/domain"
)

const secretPrefix = "secret."

var keyPattern = regexp.MustCompile(`^(prompt|param|schedule|secret)\.[a-z0-9][a-z0-9_-]{0,63}$`)

type Key string

func ParseKey(raw string) (Key, error) {
	if !keyPattern.MatchString(raw) {
		return "", domain.Error(domain.ErrValidation, "invalid setting key", slog.String("key", raw))
	}
	return Key(raw), nil
}

func (k Key) IsSecret() bool {
	return strings.HasPrefix(string(k), secretPrefix)
}
