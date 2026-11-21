package hotcache

import (
	"errors"
	"io"
	"strings"

	"claude-memory-sync/internal/domain"
	"claude-memory-sync/internal/guardrails"
)

const (
	hotBeginMarker = "<!-- HOT:BEGIN -->"
	hotEndMarker   = "<!-- HOT:END -->"
)

const (
	reasonEmptyDigest    = "empty digest"
	reasonDigestTooLarge = "digest too large"
)

type Result struct {
	Written bool   `json:"written"`
	Reason  string `json:"reason"`
}

func Write(memoryDir string, maxBytes int, in io.Reader) (Result, error) {
	digest, err := io.ReadAll(in)
	if err != nil {
		return Result{}, domain.Error(err, "read digest input failed")
	}

	if strings.TrimSpace(string(digest)) == "" {
		return Result{Written: false, Reason: reasonEmptyDigest}, nil
	}

	err = guardrails.ReplaceHotBlock(memoryDir, digest, maxBytes)
	if errors.Is(err, guardrails.ErrDigestTooLarge) {
		return Result{Written: false, Reason: reasonDigestTooLarge}, nil
	}
	if err != nil {
		return Result{}, domain.Error(err, "replace hot block failed")
	}

	return Result{Written: true, Reason: ""}, nil
}

func ExtractDigest(content string) string {
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
