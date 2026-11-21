package intake

import (
	"context"
	"os"
	"path/filepath"
	"sort"

	"claude-memory-sync/internal/domain"
	"claude-memory-sync/internal/gitutil"
	"claude-memory-sync/internal/guardrails"
)

const skipReasonSecret = "secret"

type MergeSkip struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

type MergeResult struct {
	Changed []string    `json:"changed"`
	Skipped []MergeSkip `json:"skipped"`
}

type configWinner struct {
	Path      string
	Timestamp int64
	Deleted   bool
	ClientID  string
}

type configApplyOutcome struct {
	Changed bool
	Skip    *MergeSkip
}

func MergeConfig(ctx context.Context, reposDir, workDir string, scan ScanResult) (MergeResult, error) {
	heads := clientHeads(scan)
	winners := selectConfigWinners(scan)

	result := MergeResult{Changed: make([]string, 0), Skipped: make([]MergeSkip, 0)}

	for _, winner := range winners {
		outcome, err := applyConfigWinner(ctx, reposDir, workDir, winner, heads)
		if err != nil {
			return MergeResult{}, err
		}
		if outcome.Skip != nil {
			result.Skipped = append(result.Skipped, *outcome.Skip)
			continue
		}
		if outcome.Changed {
			result.Changed = append(result.Changed, winner.Path)
		}
	}

	return result, nil
}

func clientHeads(scan ScanResult) map[string]string {
	heads := make(map[string]string, len(scan.Clients))
	for _, client := range scan.Clients {
		heads[client.ID] = client.Head
	}
	return heads
}

func selectConfigWinners(scan ScanResult) []configWinner {
	winners := make(map[string]configWinner)

	for _, client := range scan.Clients {
		for _, cand := range client.Config {
			current, exists := winners[cand.Path]
			if exists && cand.Timestamp <= current.Timestamp {
				continue
			}
			winners[cand.Path] = configWinner{
				Path:      cand.Path,
				Timestamp: cand.Timestamp,
				Deleted:   cand.Deleted,
				ClientID:  client.ID,
			}
		}
	}

	paths := make([]string, 0, len(winners))
	for path := range winners {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	result := make([]configWinner, 0, len(paths))
	for _, path := range paths {
		result = append(result, winners[path])
	}
	return result
}

func applyConfigWinner(ctx context.Context, reposDir, workDir string, winner configWinner, heads map[string]string) (configApplyOutcome, error) {
	dest := filepath.Join(workDir, filepath.FromSlash(winner.Path))

	if winner.Deleted {
		return removeConfigPath(dest)
	}

	head := heads[winner.ClientID]
	repoDir := filepath.Join(reposDir, "clients", winner.ClientID+".git")

	content, err := gitutil.RunOutput(ctx, repoDir, "show", head+":"+winner.Path)
	if err != nil {
		return configApplyOutcome{}, domain.Error(err, "git show config content failed")
	}

	if containsSecret(content) {
		return configApplyOutcome{Skip: &MergeSkip{Path: winner.Path, Reason: skipReasonSecret}}, nil
	}

	if err := os.MkdirAll(filepath.Dir(dest), 0o750); err != nil {
		return configApplyOutcome{}, domain.Error(err, "create config directory failed")
	}
	if err := os.WriteFile(dest, []byte(content), 0o600); err != nil {
		return configApplyOutcome{}, domain.Error(err, "write config file failed")
	}

	return configApplyOutcome{Changed: true}, nil
}

func containsSecret(content string) bool {
	return guardrails.ScanSecrets([]byte(content)) != nil
}

func removeConfigPath(dest string) (configApplyOutcome, error) {
	if _, err := os.Stat(dest); os.IsNotExist(err) {
		return configApplyOutcome{}, nil
	}
	if err := os.Remove(dest); err != nil {
		return configApplyOutcome{}, domain.Error(err, "remove config file failed")
	}
	return configApplyOutcome{Changed: true}, nil
}
