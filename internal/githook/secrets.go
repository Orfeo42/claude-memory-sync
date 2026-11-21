package githook

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"strconv"

	"claude-memory-sync/internal/domain"
	"claude-memory-sync/internal/gitutil"
	"claude-memory-sync/internal/hiddentext"
)

const maxScannedFileSize = 1 << 20

// secretPattern: keep in sync with build/synthesizer secret_pattern (build/synthesizer/lib.sh).
var secretPattern = regexp.MustCompile(`(?i)(api[_-]?key|secret|password|token)\s*[:=]\s*[A-Za-z0-9_-]{16,}|\bghp_[A-Za-z0-9]{20,}|\bsk-[A-Za-z0-9_-]{20,}|\bAKIA[A-Z0-9]{16}|-----BEGIN [A-Z ]*PRIVATE KEY-----`)

func ContainsSecret(data []byte) bool {
	return secretPattern.Match(data)
}

func scanContent(ctx context.Context, gitDir, newSHA string, entries []diffEntry) error {
	for _, entry := range entries {
		if entry.deleted {
			continue
		}

		if err := scanBlob(ctx, gitDir, newSHA, entry.path); err != nil {
			return err
		}
	}

	return nil
}

func scanBlob(ctx context.Context, gitDir, newSHA, path string) error {
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

	if finding, found := hiddentext.Find([]byte(content)); found {
		return domain.Error(errHiddenCharacter, fmt.Sprintf("path %q contains hidden character %s at byte %d", path, finding.CodePoint(), finding.Offset),
			slog.String("path", path), slog.Int("offset", finding.Offset), slog.String("code_point", finding.CodePoint()))
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
