package main

import (
	"context"
	"encoding/json"
	"flag"
	"os"

	"claude-memory-sync/internal/corpus"
	"claude-memory-sync/internal/domain"
	"claude-memory-sync/internal/intake"
)

const errUnknownCorpusSubcommand cliError = "unknown corpus subcommand"

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

	summaries, err := corpus.ListProjects(ctx, *workDir)
	if err != nil {
		return err
	}

	if err := json.NewEncoder(os.Stdout).Encode(summaries); err != nil {
		return domain.Error(err, "encode corpus list result failed")
	}
	return nil
}

func cmdCorpusRead(args []string) error {
	fs := flag.NewFlagSet("corpus read", flag.ExitOnError)
	workDir := fs.String("work-dir", "", "work directory")
	project := fs.String("project", "", "project key")
	file := fs.String("file", "MEMORY.md", "entry file name")
	if err := fs.Parse(args); err != nil {
		return domain.Error(err, "parse corpus read flags failed")
	}

	return corpus.ReadEntry(os.Stdout, *workDir, *project, *file)
}

func cmdCorpusSearch(args []string) error {
	fs := flag.NewFlagSet("corpus search", flag.ExitOnError)
	workDir := fs.String("work-dir", "", "work directory")
	query := fs.String("query", "", "search query")
	if err := fs.Parse(args); err != nil {
		return domain.Error(err, "parse corpus search flags failed")
	}

	hits, err := corpus.Search(*workDir, *query)
	if err != nil {
		return err
	}

	if err := json.NewEncoder(os.Stdout).Encode(hits); err != nil {
		return domain.Error(err, "encode corpus search result failed")
	}
	return nil
}

func cmdCorpusDigest(args []string) error {
	fs := flag.NewFlagSet("corpus digest", flag.ExitOnError)
	workDir := fs.String("work-dir", "", "work directory")
	project := fs.String("project", "", "project key")
	if err := fs.Parse(args); err != nil {
		return domain.Error(err, "parse corpus digest flags failed")
	}

	return corpus.WriteDigest(os.Stdout, *workDir, *project)
}

func cmdContext(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("context", flag.ExitOnError)
	reposDir := fs.String("repos-dir", "", "repos directory")
	workDir := fs.String("work-dir", "", "work directory")
	project := fs.String("project", "", "project key")
	if err := fs.Parse(args); err != nil {
		return domain.Error(err, "parse context flags failed")
	}

	var scan intake.ScanResult
	if err := json.NewDecoder(os.Stdin).Decode(&scan); err != nil {
		return domain.Error(err, "decode scan input failed")
	}

	return corpus.WriteContext(ctx, os.Stdout, *reposDir, *workDir, *project, scan)
}
