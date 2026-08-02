package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"claude-memory-sync/internal/domain"
	"claude-memory-sync/internal/gitutil"
)

func cmdContext(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("context", flag.ExitOnError)
	reposDir := fs.String("repos-dir", "", "repos directory")
	workDir := fs.String("work-dir", "", "work directory")
	project := fs.String("project", "", "project key")
	if err := fs.Parse(args); err != nil {
		return domain.Error(err, "parse context flags failed")
	}

	var scan scanResult
	if err := json.NewDecoder(os.Stdin).Decode(&scan); err != nil {
		return domain.Error(err, "decode scan input failed")
	}

	return writeContext(ctx, os.Stdout, *reposDir, *workDir, *project, scan)
}

func writeContext(ctx context.Context, out io.Writer, reposDir, workDir, project string, scan scanResult) error {
	if err := writeContextIndex(out, workDir, project); err != nil {
		return err
	}
	if err := writeContextEntries(out, workDir, project); err != nil {
		return err
	}
	return writeContextCandidates(ctx, out, reposDir, project, scan)
}

func writeContextIndex(out io.Writer, workDir, project string) error {
	indexPath := filepath.Join(workDir, "projects", project, "memory", "MEMORY.md")

	content, err := os.ReadFile(indexPath)
	if err != nil && !os.IsNotExist(err) {
		return domain.Error(err, "read project index failed")
	}

	if _, err := fmt.Fprintf(out, "INDEX:\n%s\n\nENTRIES:\n", strings.TrimRight(string(content), "\n")); err != nil {
		return domain.Error(err, "write context index failed")
	}
	return nil
}

func writeContextEntries(out io.Writer, workDir, project string) error {
	memoryDir := filepath.Join(workDir, "projects", project, "memory")

	names, err := corpusEntryNames(memoryDir)
	if err != nil {
		return err
	}

	for _, name := range names {
		content, err := os.ReadFile(filepath.Join(memoryDir, name))
		if err != nil {
			return domain.Error(err, "read memory entry failed")
		}
		if err := writeContextBlock(out, name, content); err != nil {
			return err
		}
	}

	if _, err := fmt.Fprint(out, "\n"); err != nil {
		return domain.Error(err, "write context separator failed")
	}
	return nil
}

func writeContextBlock(out io.Writer, name string, content []byte) error {
	trimmed := strings.TrimRight(string(content), "\n")
	if _, err := fmt.Fprintf(out, "--- %s ---\n%s\n", name, trimmed); err != nil {
		return domain.Error(err, "write context block failed")
	}
	return nil
}

func writeContextCandidates(ctx context.Context, out io.Writer, reposDir, project string, scan scanResult) error {
	for _, client := range scan.Clients {
		candidates := filterMemoryCandidates(client.Memory, project)
		if len(candidates) == 0 {
			continue
		}

		if _, err := fmt.Fprintf(out, "CANDIDATES (from machine %s):\n", client.ID); err != nil {
			return domain.Error(err, "write candidates header failed")
		}

		if err := writeCandidateBlocks(ctx, out, reposDir, client, candidates); err != nil {
			return err
		}
	}
	return nil
}

func filterMemoryCandidates(candidates []memoryCandidate, project string) []memoryCandidate {
	filtered := make([]memoryCandidate, 0, len(candidates))
	for _, cand := range candidates {
		if cand.Project == project {
			filtered = append(filtered, cand)
		}
	}
	return filtered
}

func writeCandidateBlocks(ctx context.Context, out io.Writer, reposDir string, client clientScan, candidates []memoryCandidate) error {
	repoDir := filepath.Join(reposDir, "clients", client.ID+".git")

	for _, cand := range candidates {
		content, err := gitutil.RunOutput(ctx, repoDir, "show", client.Head+":"+cand.Path)
		if err != nil {
			return domain.Error(err, "git show memory candidate failed")
		}

		basename := filepath.Base(cand.Path)
		if err := writeContextBlock(out, basename, []byte(content)); err != nil {
			return err
		}
	}
	return nil
}
