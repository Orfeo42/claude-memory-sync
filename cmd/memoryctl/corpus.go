package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"claude-memory-sync/internal/domain"
	"claude-memory-sync/internal/gitutil"
)

const (
	hotBeginMarker = "<!-- HOT:BEGIN -->"
	hotEndMarker   = "<!-- HOT:END -->"
)

const errUnknownCorpusSubcommand cliError = "unknown corpus subcommand"

const errInvalidCorpusFile cliError = "invalid corpus file name"

type corpusProjectSummary struct {
	Key        string `json:"key"`
	Entries    int    `json:"entries"`
	LastCommit string `json:"lastCommit"`
}

type corpusSearchHit struct {
	Project string `json:"project"`
	File    string `json:"file"`
	Line    int    `json:"line"`
	Text    string `json:"text"`
}

func cmdCorpus(ctx context.Context, args []string) error {
	if len(args) < 1 {
		return domain.Error(errUnknownCorpusSubcommand, "corpus subcommand required")
	}

	switch args[0] {
	case "list":
		return cmdCorpusList(ctx, args[1:])
	case "read":
		return cmdCorpusRead(args[1:])
	case "search":
		return cmdCorpusSearch(args[1:])
	case "digest":
		return cmdCorpusDigest(args[1:])
	default:
		return domain.Error(errUnknownCorpusSubcommand, "unrecognized corpus subcommand")
	}
}

func cmdCorpusList(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("corpus list", flag.ExitOnError)
	workDir := fs.String("work-dir", "", "work directory")
	if err := fs.Parse(args); err != nil {
		return domain.Error(err, "parse corpus list flags failed")
	}

	summaries, err := listCorpusProjects(ctx, *workDir)
	if err != nil {
		return err
	}

	if err := json.NewEncoder(os.Stdout).Encode(summaries); err != nil {
		return domain.Error(err, "encode corpus list result failed")
	}
	return nil
}

func listCorpusProjects(ctx context.Context, workDir string) ([]corpusProjectSummary, error) {
	keys, err := corpusProjectKeys(workDir)
	if err != nil {
		return nil, err
	}

	summaries := make([]corpusProjectSummary, 0, len(keys))
	for _, key := range keys {
		summary, err := buildCorpusProjectSummary(ctx, workDir, key)
		if err != nil {
			return nil, err
		}
		summaries = append(summaries, summary)
	}
	return summaries, nil
}

func corpusProjectKeys(workDir string) ([]string, error) {
	entries, err := os.ReadDir(filepath.Join(workDir, "projects"))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, domain.Error(err, "read projects directory failed")
	}

	keys := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		memoryDir := filepath.Join(workDir, "projects", entry.Name(), "memory")
		if _, statErr := os.Stat(memoryDir); statErr != nil {
			continue
		}
		keys = append(keys, entry.Name())
	}

	sort.Strings(keys)
	return keys, nil
}

func buildCorpusProjectSummary(ctx context.Context, workDir, key string) (corpusProjectSummary, error) {
	memoryDir := filepath.Join(workDir, "projects", key, "memory")

	names, err := corpusEntryNames(memoryDir)
	if err != nil {
		return corpusProjectSummary{}, err
	}

	logPath := filepath.ToSlash(filepath.Join("projects", key, "memory"))
	lastCommit, err := gitutil.RunOutput(ctx, workDir, "log", "-1", "--format=%cI", "--", logPath)
	if err != nil {
		lastCommit = ""
	}

	return corpusProjectSummary{Key: key, Entries: len(names), LastCommit: lastCommit}, nil
}

func corpusEntryNames(memoryDir string) ([]string, error) {
	entries, err := os.ReadDir(memoryDir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, domain.Error(err, "read memory directory failed")
	}

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || entry.Name() == "MEMORY.md" || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		names = append(names, entry.Name())
	}

	sort.Strings(names)
	return names, nil
}

func cmdCorpusRead(args []string) error {
	fs := flag.NewFlagSet("corpus read", flag.ExitOnError)
	workDir := fs.String("work-dir", "", "work directory")
	project := fs.String("project", "", "project key")
	file := fs.String("file", "MEMORY.md", "entry file name")
	if err := fs.Parse(args); err != nil {
		return domain.Error(err, "parse corpus read flags failed")
	}

	return readCorpusEntry(os.Stdout, *workDir, *project, *file)
}

func readCorpusEntry(out io.Writer, workDir, project, file string) error {
	if strings.Contains(file, "/") || strings.Contains(file, "..") {
		return domain.Error(errInvalidCorpusFile, "corpus file name is unsafe")
	}

	path := filepath.Join(workDir, "projects", project, "memory", file)
	content, err := os.ReadFile(path)
	if err != nil {
		return domain.Error(err, "read corpus entry failed")
	}

	if _, err := out.Write(content); err != nil {
		return domain.Error(err, "write corpus entry output failed")
	}
	return nil
}

func cmdCorpusSearch(args []string) error {
	fs := flag.NewFlagSet("corpus search", flag.ExitOnError)
	workDir := fs.String("work-dir", "", "work directory")
	query := fs.String("query", "", "search query")
	if err := fs.Parse(args); err != nil {
		return domain.Error(err, "parse corpus search flags failed")
	}

	hits, err := searchCorpus(*workDir, *query)
	if err != nil {
		return err
	}

	if err := json.NewEncoder(os.Stdout).Encode(hits); err != nil {
		return domain.Error(err, "encode corpus search result failed")
	}
	return nil
}

func searchCorpus(workDir, query string) ([]corpusSearchHit, error) {
	keys, err := corpusProjectKeys(workDir)
	if err != nil {
		return nil, err
	}

	hits := make([]corpusSearchHit, 0)
	lowerQuery := strings.ToLower(query)

	for _, key := range keys {
		memoryDir := filepath.Join(workDir, "projects", key, "memory")
		names, err := corpusEntryNames(memoryDir)
		if err != nil {
			return nil, err
		}

		for _, name := range names {
			fileHits, err := searchCorpusFile(filepath.Join(memoryDir, name), key, name, lowerQuery)
			if err != nil {
				return nil, err
			}
			hits = append(hits, fileHits...)
		}
	}

	return hits, nil
}

func searchCorpusFile(path, project, name, lowerQuery string) ([]corpusSearchHit, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, domain.Error(err, "open corpus entry failed")
	}
	defer func() {
		_ = file.Close()
	}()

	hits := make([]corpusSearchHit, 0)
	scanner := bufio.NewScanner(file)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		if strings.Contains(strings.ToLower(line), lowerQuery) {
			hits = append(hits, corpusSearchHit{Project: project, File: name, Line: lineNum, Text: line})
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, domain.Error(err, "scan corpus entry failed")
	}

	return hits, nil
}

func cmdCorpusDigest(args []string) error {
	fs := flag.NewFlagSet("corpus digest", flag.ExitOnError)
	workDir := fs.String("work-dir", "", "work directory")
	project := fs.String("project", "", "project key")
	if err := fs.Parse(args); err != nil {
		return domain.Error(err, "parse corpus digest flags failed")
	}

	return writeCorpusDigest(os.Stdout, *workDir, *project)
}

func writeCorpusDigest(out io.Writer, workDir, project string) error {
	path := filepath.Join(workDir, "projects", project, "memory", "MEMORY.md")

	content, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return domain.Error(err, "read corpus index failed")
	}

	digest := extractHotDigest(string(content))
	if digest == "" {
		return nil
	}

	if _, err := fmt.Fprintln(out, digest); err != nil {
		return domain.Error(err, "write corpus digest output failed")
	}
	return nil
}

func extractHotDigest(content string) string {
	lines := strings.Split(content, "\n")
	beginIdx, endIdx := -1, -1
	for i, line := range lines {
		if line == hotBeginMarker {
			beginIdx = i
		}
		if line == hotEndMarker {
			endIdx = i
		}
	}
	if beginIdx == -1 || endIdx == -1 || endIdx <= beginIdx {
		return ""
	}
	return strings.Join(lines[beginIdx+1:endIdx], "\n")
}
