package guardrails

import "claude-memory-sync/internal/githook"

func ScanSecrets(content []byte) error {
	if githook.ContainsSecret(content) {
		return ErrSecretDetected
	}

	return nil
}
