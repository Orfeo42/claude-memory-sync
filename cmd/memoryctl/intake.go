package main

import (
	"context"
	"encoding/json"
	"flag"
	"os"

	"claude-memory-sync/internal/domain"
	"claude-memory-sync/internal/intake"
)

func cmdClientsScan(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("clients-scan", flag.ExitOnError)
	reposDir := fs.String("repos-dir", "", "repos directory")
	if err := fs.Parse(args); err != nil {
		return domain.Error(err, "parse clients-scan flags failed")
	}

	result, err := intake.Scan(ctx, *reposDir)
	if err != nil {
		return err
	}

	if err := json.NewEncoder(os.Stdout).Encode(result); err != nil {
		return domain.Error(err, "encode clients-scan result failed")
	}
	return nil
}

func cmdConfigMerge(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("config-merge", flag.ExitOnError)
	reposDir := fs.String("repos-dir", "", "repos directory")
	workDir := fs.String("work-dir", "", "work directory")
	if err := fs.Parse(args); err != nil {
		return domain.Error(err, "parse config-merge flags failed")
	}

	var scan intake.ScanResult
	if err := json.NewDecoder(os.Stdin).Decode(&scan); err != nil {
		return domain.Error(err, "decode scan input failed")
	}

	result, err := intake.MergeConfig(ctx, *reposDir, *workDir, scan)
	if err != nil {
		return err
	}

	if err := json.NewEncoder(os.Stdout).Encode(result); err != nil {
		return domain.Error(err, "encode config-merge result failed")
	}
	return nil
}

func cmdMarkersAdvance(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("markers-advance", flag.ExitOnError)
	reposDir := fs.String("repos-dir", "", "repos directory")
	client := fs.String("client", "", "client id")
	to := fs.String("to", "", "target sha")
	if err := fs.Parse(args); err != nil {
		return domain.Error(err, "parse markers-advance flags failed")
	}

	return intake.AdvanceMarker(ctx, *reposDir, *client, *to)
}
