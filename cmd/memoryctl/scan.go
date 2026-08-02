package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"claude-memory-sync/internal/domain"
	"claude-memory-sync/internal/gitutil"
)

const emptyTreeSHA = "4b825dc642cb6eb9a060e54bf8d69288fbee4904"

const errClientHeadMissing cliError = "client head missing"

type configCandidate struct {
	Path      string `json:"path"`
	Timestamp int64  `json:"timestamp"`
	Deleted   bool   `json:"deleted"`
}

type memoryCandidate struct {
	Project string `json:"project"`
	Path    string `json:"path"`
}

type clientScan struct {
	ID     string            `json:"id"`
	Head   string            `json:"head"`
	Marker string            `json:"marker"`
	Config []configCandidate `json:"config"`
	Memory []memoryCandidate `json:"memory"`
}

type scanResult struct {
	Clients []clientScan `json:"clients"`
}

type diffLine struct {
	Status string
	Path   string
	Valid  bool
}

type memoryPathInfo struct {
	Project  string
	Basename string
	Matched  bool
}

type diffScan struct {
	Config []configCandidate
	Memory []memoryCandidate
}

func cmdClientsScan(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("clients-scan", flag.ExitOnError)
	reposDir := fs.String("repos-dir", "", "repos directory")
	if err := fs.Parse(args); err != nil {
		return domain.Error(err, "parse clients-scan flags failed")
	}

	result, err := scanClients(ctx, *reposDir)
	if err != nil {
		return err
	}

	if err := json.NewEncoder(os.Stdout).Encode(result); err != nil {
		return domain.Error(err, "encode clients-scan result failed")
	}
	return nil
}

func scanClients(ctx context.Context, reposDir string) (scanResult, error) {
	dirs, err := clientRepoDirs(reposDir)
	if err != nil {
		return scanResult{}, err
	}

	clients := make([]clientScan, 0, len(dirs))
	for _, dir := range dirs {
		client, err := scanClientRepo(ctx, reposDir, dir)
		if errors.Is(err, errClientHeadMissing) {
			continue
		}
		if err != nil {
			return scanResult{}, err
		}
		clients = append(clients, *client)
	}

	return scanResult{Clients: clients}, nil
}

func clientRepoDirs(reposDir string) ([]string, error) {
	entries, err := os.ReadDir(filepath.Join(reposDir, "clients"))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, domain.Error(err, "read clients directory failed")
	}

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasSuffix(entry.Name(), ".git") {
			continue
		}
		names = append(names, entry.Name())
	}

	sort.Strings(names)
	return names, nil
}

func scanClientRepo(ctx context.Context, reposDir, dirName string) (*clientScan, error) {
	repoDir := filepath.Join(reposDir, "clients", dirName)
	id := strings.TrimSuffix(dirName, ".git")

	head, _ := gitRevParse(ctx, repoDir, "refs/heads/main")
	if head == "" {
		return nil, errClientHeadMissing
	}

	marker, _ := gitRevParse(ctx, repoDir, "refs/intake/last-processed")

	client := clientScan{
		ID:     id,
		Head:   head,
		Marker: marker,
		Config: make([]configCandidate, 0),
		Memory: make([]memoryCandidate, 0),
	}

	if marker != "" && marker == head {
		return &client, nil
	}

	base := marker
	if base == "" {
		base = emptyTreeSHA
	}

	diff, err := computeDiffCandidates(ctx, repoDir, base, head)
	if err != nil {
		return nil, err
	}

	client.Config = diff.Config
	client.Memory = diff.Memory
	return &client, nil
}

func gitRevParse(ctx context.Context, repoDir, ref string) (string, error) {
	return gitutil.RunOutput(ctx, repoDir, "rev-parse", "--verify", "--quiet", ref)
}

func computeDiffCandidates(ctx context.Context, repoDir, base, head string) (diffScan, error) {
	out, err := gitutil.RunOutput(ctx, repoDir, "diff", "--name-status", base, head)
	if err != nil {
		return diffScan{}, domain.Error(err, "git diff failed")
	}

	result := diffScan{Config: make([]configCandidate, 0), Memory: make([]memoryCandidate, 0)}

	for _, raw := range strings.Split(out, "\n") {
		if raw == "" {
			continue
		}

		line := parseDiffStatusLine(raw)
		if !line.Valid {
			continue
		}

		if err := classifyDiffLine(ctx, repoDir, head, line, &result); err != nil {
			return diffScan{}, err
		}
	}

	return result, nil
}

func classifyDiffLine(ctx context.Context, repoDir, head string, line diffLine, result *diffScan) error {
	if isConfigPath(line.Path) {
		cand, err := buildConfigCandidate(ctx, repoDir, head, line)
		if err != nil {
			return err
		}
		result.Config = append(result.Config, cand)
		return nil
	}

	info := parseMemoryPath(line.Path)
	if info.Matched && !strings.HasPrefix(line.Status, "D") {
		result.Memory = append(result.Memory, memoryCandidate{Project: info.Project, Path: line.Path})
	}
	return nil
}

func parseDiffStatusLine(line string) diffLine {
	fields := strings.Split(line, "\t")
	if len(fields) < 2 {
		return diffLine{}
	}
	return diffLine{Status: fields[0], Path: fields[1], Valid: true}
}

func isConfigPath(path string) bool {
	return strings.HasPrefix(path, "global/")
}

func parseMemoryPath(path string) memoryPathInfo {
	segments := strings.Split(path, "/")
	if len(segments) != 4 || segments[0] != "projects" || segments[2] != "memory" {
		return memoryPathInfo{}
	}
	if !strings.HasSuffix(segments[3], ".md") || segments[3] == "MEMORY.md" {
		return memoryPathInfo{}
	}
	return memoryPathInfo{Project: segments[1], Basename: segments[3], Matched: true}
}

func buildConfigCandidate(ctx context.Context, repoDir, head string, line diffLine) (configCandidate, error) {
	ts, err := gitCommitTimestamp(ctx, repoDir, head, line.Path)
	if err != nil {
		return configCandidate{}, err
	}
	return configCandidate{Path: line.Path, Timestamp: ts, Deleted: strings.HasPrefix(line.Status, "D")}, nil
}

func gitCommitTimestamp(ctx context.Context, repoDir, head, path string) (int64, error) {
	out, err := gitutil.RunOutput(ctx, repoDir, "log", "-1", "--format=%ct", head, "--", path)
	if err != nil {
		return 0, domain.Error(err, "git log timestamp failed")
	}
	if out == "" {
		return 0, nil
	}

	ts, err := strconv.ParseInt(out, 10, 64)
	if err != nil {
		return 0, domain.Error(err, "parse commit timestamp failed")
	}
	return ts, nil
}
