package githook

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"

	"claude-memory-sync/internal/domain"
	"claude-memory-sync/internal/gitutil"
)

const (
	mainRef      = "refs/heads/main"
	zeroSHA      = "0000000000000000000000000000000000000000"
	emptyTreeSHA = "4b825dc642cb6eb9a060e54bf8d69288fbee4904"

	allowedFileMode = "100644"
	allowedExecMode = "100755"
	deletedStatus   = "D"
)

type refUpdate struct {
	oldSHA string
	newSHA string
	ref    string
}

type diffEntry struct {
	path    string
	newMode string
	deleted bool
}

func Validate(ctx context.Context, gitDir string, stdin io.Reader) error {
	updates, err := parseUpdates(stdin)
	if err != nil {
		return err
	}

	for _, update := range updates {
		if err := validateUpdate(ctx, gitDir, update); err != nil {
			return err
		}
	}

	return nil
}

func parseUpdates(stdin io.Reader) ([]refUpdate, error) {
	var updates []refUpdate

	scanner := bufio.NewScanner(stdin)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) != 3 {
			return nil, domain.Error(errMalformedInput, fmt.Sprintf("expected 3 fields, got %d: %q", len(fields), line))
		}

		updates = append(updates, refUpdate{oldSHA: fields[0], newSHA: fields[1], ref: fields[2]})
	}

	if err := scanner.Err(); err != nil {
		return nil, domain.Error(err, "failed to read pre-receive input")
	}

	return updates, nil
}

func validateUpdate(ctx context.Context, gitDir string, update refUpdate) error {
	if update.ref != mainRef {
		return domain.Error(errInvalidRef, fmt.Sprintf("only %s may be pushed, got %q", mainRef, update.ref))
	}

	if update.newSHA == zeroSHA {
		return domain.Error(errBranchDeletion, fmt.Sprintf("deleting %q is not allowed", update.ref))
	}

	base := update.oldSHA
	if base == zeroSHA {
		base = emptyTreeSHA
	}

	entries, err := diffEntries(ctx, gitDir, base, update.newSHA)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if err := checkWhitelist(entry.path); err != nil {
			return err
		}
	}

	for _, entry := range entries {
		if entry.deleted {
			continue
		}

		if err := checkMode(entry); err != nil {
			return err
		}
	}

	return scanSecrets(ctx, gitDir, update.newSHA, entries)
}

func diffEntries(ctx context.Context, gitDir, base, newSHA string) ([]diffEntry, error) {
	out, err := gitutil.RunOutput(ctx, gitDir, "diff", "--raw", base, newSHA)
	if err != nil {
		return nil, domain.Error(err, "failed to compute changed paths")
	}

	if out == "" {
		return nil, nil
	}

	var entries []diffEntry
	for _, line := range strings.Split(out, "\n") {
		entry, ok := parseDiffRawLine(line)
		if !ok {
			continue
		}

		entries = append(entries, entry)
	}

	return entries, nil
}

func parseDiffRawLine(line string) (diffEntry, bool) {
	fields := strings.Fields(line)
	if len(fields) < 6 {
		return diffEntry{}, false
	}

	return diffEntry{
		path:    fields[len(fields)-1],
		newMode: fields[1],
		deleted: strings.HasPrefix(fields[4], deletedStatus),
	}, true
}

func checkMode(entry diffEntry) error {
	if entry.newMode == allowedFileMode || entry.newMode == allowedExecMode {
		return nil
	}

	return domain.Error(errDisallowedMode, fmt.Sprintf("path %q has disallowed mode %s", entry.path, entry.newMode))
}
