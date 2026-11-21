package corpus

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"claude-memory-sync/internal/domain"
	"claude-memory-sync/internal/gitutil"
	"claude-memory-sync/internal/hotcache"
)

const memoryIndexFile = "MEMORY.md"

type corpusError string

func (e corpusError) Error() string { return string(e) }

const errInvalidCorpusFile corpusError = "invalid corpus file name"

type ProjectSummary struct {
	Key        string `json:"key"`
	Entries    int    `json:"entries"`
	LastCommit string `json:"lastCommit"`
}

type SearchHit struct {
	Project string `json:"project"`
	File    string `json:"file"`
	Line    int    `json:"line"`
	Text    string `json:"text"`
}

func ListProjects(ctx context.Context, workDir string) ([]ProjectSummary, error) {
	keys, err := corpusProjectKeys(workDir)
	if err != nil {
		return nil, err
	}

	summaries := make([]ProjectSummary, 0, len(keys))
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

func buildCorpusProjectSummary(ctx context.Context, workDir, key string) (ProjectSummary, error) {
	memoryDir := filepath.Join(workDir, "projects", key, "memory")

	names, err := corpusEntryNames(memoryDir)
	if err != nil {
		return ProjectSummary{}, err
	}

	logPath := filepath.ToSlash(filepath.Join("projects", key, "memory"))
	lastCommit, err := gitutil.RunOutput(ctx, workDir, "log", "-1", "--format=%cI", "--", logPath)
	if err != nil {
		lastCommit = ""
	}

	return ProjectSummary{Key: key, Entries: len(names), LastCommit: lastCommit}, nil
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
		if entry.IsDir() || entry.Name() == memoryIndexFile || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		names = append(names, entry.Name())
	}

	sort.Strings(names)
	return names, nil
}

func ReadEntry(out io.Writer, workDir, project, file string) error {
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

func Search(workDir, query string) ([]SearchHit, error) {
	keys, err := corpusProjectKeys(workDir)
	if err != nil {
		return nil, err
	}

	hits := make([]SearchHit, 0)
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

func searchCorpusFile(path, project, name, lowerQuery string) ([]SearchHit, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, domain.Error(err, "open corpus entry failed")
	}
	defer func() {
		_ = file.Close()
	}()

	hits := make([]SearchHit, 0)
	scanner := bufio.NewScanner(file)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		if strings.Contains(strings.ToLower(line), lowerQuery) {
			hits = append(hits, SearchHit{Project: project, File: name, Line: lineNum, Text: line})
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, domain.Error(err, "scan corpus entry failed")
	}

	return hits, nil
}

func WriteDigest(out io.Writer, workDir, project string) error {
	path := filepath.Join(workDir, "projects", project, "memory", "MEMORY.md")

	content, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return domain.Error(err, "read corpus index failed")
	}

	digest := hotcache.ExtractDigest(string(content))
	if digest == "" {
		return nil
	}

	if _, err := fmt.Fprintln(out, digest); err != nil {
		return domain.Error(err, "write corpus digest output failed")
	}
	return nil
}
