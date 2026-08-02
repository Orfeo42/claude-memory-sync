package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"os"
	"strings"

	"claude-memory-sync/internal/domain"
	"claude-memory-sync/internal/guardrails"
)

const (
	reasonEmptyDigest    = "empty digest"
	reasonDigestTooLarge = "digest too large"
)

type hotcacheResult struct {
	Written bool   `json:"written"`
	Reason  string `json:"reason"`
}

func cmdHotcacheWrite(_ context.Context, args []string) error {
	fs := flag.NewFlagSet("hotcache-write", flag.ExitOnError)
	memoryDir := fs.String("memory-dir", "", "memory directory")
	maxBytes := fs.Int("max-bytes", 0, "max digest bytes")
	if err := fs.Parse(args); err != nil {
		return domain.Error(err, "parse hotcache-write flags failed")
	}

	result, err := writeHotCache(*memoryDir, *maxBytes, os.Stdin)
	if err != nil {
		return err
	}

	if err := json.NewEncoder(os.Stdout).Encode(result); err != nil {
		return domain.Error(err, "encode hotcache-write result failed")
	}
	return nil
}

func writeHotCache(memoryDir string, maxBytes int, in io.Reader) (hotcacheResult, error) {
	digest, err := io.ReadAll(in)
	if err != nil {
		return hotcacheResult{}, domain.Error(err, "read digest input failed")
	}

	if strings.TrimSpace(string(digest)) == "" {
		return hotcacheResult{Written: false, Reason: reasonEmptyDigest}, nil
	}

	err = guardrails.ReplaceHotBlock(memoryDir, digest, maxBytes)
	if errors.Is(err, guardrails.ErrDigestTooLarge) {
		return hotcacheResult{Written: false, Reason: reasonDigestTooLarge}, nil
	}
	if err != nil {
		return hotcacheResult{}, domain.Error(err, "replace hot block failed")
	}

	return hotcacheResult{Written: true, Reason: ""}, nil
}
