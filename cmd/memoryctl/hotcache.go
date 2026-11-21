package main

import (
	"context"
	"encoding/json"
	"flag"
	"os"

	"claude-memory-sync/internal/domain"
	"claude-memory-sync/internal/hotcache"
)

func cmdHotcacheWrite(_ context.Context, args []string) error {
	fs := flag.NewFlagSet("hotcache-write", flag.ExitOnError)
	memoryDir := fs.String("memory-dir", "", "memory directory")
	maxBytes := fs.Int("max-bytes", 0, "max digest bytes")
	if err := fs.Parse(args); err != nil {
		return domain.Error(err, "parse hotcache-write flags failed")
	}

	result, err := hotcache.Write(*memoryDir, *maxBytes, os.Stdin)
	if err != nil {
		return err
	}

	if err := json.NewEncoder(os.Stdout).Encode(result); err != nil {
		return domain.Error(err, "encode hotcache-write result failed")
	}
	return nil
}
