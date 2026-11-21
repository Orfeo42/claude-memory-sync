package guardrails

import (
	"fmt"
	"log/slog"

	"claude-memory-sync/internal/domain"
	"claude-memory-sync/internal/githook"
	"claude-memory-sync/internal/hiddentext"
)

func ScanSecrets(content []byte) error {
	if githook.ContainsSecret(content) {
		return ErrSecretDetected
	}

	return nil
}

func ScanHidden(content []byte) error {
	finding, found := hiddentext.Find(content)
	if !found {
		return nil
	}

	return domain.Error(ErrHiddenCharacter, fmt.Sprintf("hidden character %s at byte %d", finding.CodePoint(), finding.Offset),
		slog.Int("offset", finding.Offset), slog.String("code_point", finding.CodePoint()))
}
