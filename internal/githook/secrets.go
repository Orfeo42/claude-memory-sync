package githook

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"strconv"

	"claude-memory-sync/internal/domain"
	"claude-memory-sync/internal/gitutil"
)

const maxScannedFileSize = 1 << 20

// secretPattern: keep in sync with build/synthesizer secret_pattern (build/synthesizer/synth.sh).
var secretPattern = regexp.MustCompile(`(?i)(api[_-]?key|secret|password|token)\s*[:=]\s*\S{8,}|ghp_[A-Za-z0-9]{20,}|sk-[A-Za-z0-9_-]{20,}|AKIA[A-Z0-9]{16}|-----BEGIN [A-Z ]*PRIVATE KEY-----`)

func scanSecrets(ctx context.Context, gitDir, newSHA string, entries []diffEntry) error {
	for _, entry := range entries {
		if entry.deleted {
			continue
		}

		if err := scanSecretsInFile(ctx, gitDir, newSHA, entry.path); err != nil {
			return err
		}
	}

	return nil
}

func scanSecretsInFile(ctx context.Context, gitDir, newSHA, path string) error {
	size, err := blobSize(ctx, gitDir, newSHA, path)
	if err != nil {
		return domain.Error(err, "failed to stat blob size", slog.String("path", path))
	}

	if size > maxScannedFileSize {
		return nil
	}

	content, err := gitutil.RunOutput(ctx, gitDir, "show", newSHA+":"+path)
	if err != nil {
		return domain.Error(err, "failed to read file content for secret scan", slog.String("path", path))
	}

	if secretPattern.MatchString(content) {
		return domain.Error(errSecretDetected, fmt.Sprintf("path %q appears to contain a secret", path), slog.String("path", path))
	}

	return nil
}

func blobSize(ctx context.Context, gitDir, newSHA, path string) (int64, error) {
	out, err := gitutil.RunOutput(ctx, gitDir, "cat-file", "-s", newSHA+":"+path)
	if err != nil {
		return 0, err
	}

	size, err := strconv.ParseInt(out, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse blob size %q: %w", out, err)
	}

	return size, nil
}
