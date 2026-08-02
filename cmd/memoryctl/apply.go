package main

import (
	"context"
	"encoding/json"
	"flag"
	"io"
	"os"
	pathpkg "path"
	"path/filepath"
	"strings"

	"claude-memory-sync/internal/domain"
	"claude-memory-sync/internal/guardrails"
	"claude-memory-sync/internal/ops"
)

const (
	reasonDeletesNotAllowed   = "deletes not allowed"
	reasonNoSuchEntry         = "no such entry"
	reasonProposalsNotAllowed = "proposals not allowed"
	reasonInvalidProposalPath = "invalid proposal path"
)

type rejectedOp struct {
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

type applyResult struct {
	Applied   []string     `json:"applied"`
	Deleted   []string     `json:"deleted"`
	Proposals []string     `json:"proposals"`
	Rejected  []rejectedOp `json:"rejected"`
	Nothing   bool         `json:"nothing"`
}

func cmdOpsApply(_ context.Context, args []string) error {
	fs := flag.NewFlagSet("ops-apply", flag.ExitOnError)
	memoryDir := fs.String("memory-dir", "", "memory directory")
	allowDelete := fs.Bool("allow-delete", false, "allow delete ops")
	proposalsDir := fs.String("proposals-dir", "", "proposals directory")
	if err := fs.Parse(args); err != nil {
		return domain.Error(err, "parse ops-apply flags failed")
	}

	result, err := applyOps(*memoryDir, *allowDelete, *proposalsDir, os.Stdin)
	if err != nil {
		return err
	}

	if err := json.NewEncoder(os.Stdout).Encode(result); err != nil {
		return domain.Error(err, "encode ops-apply result failed")
	}
	return nil
}

func applyOps(memoryDir string, allowDelete bool, proposalsDir string, in io.Reader) (applyResult, error) {
	parsed, err := ops.Parse(in)
	if err != nil {
		return applyResult{}, domain.Error(err, "parse ops output failed")
	}

	result := applyResult{
		Applied:   make([]string, 0, len(parsed.Files)),
		Deleted:   make([]string, 0, len(parsed.Deletes)),
		Proposals: make([]string, 0, len(parsed.Proposals)),
		Rejected:  make([]rejectedOp, 0),
	}

	if parsed.Nothing {
		result.Nothing = true
		return result, nil
	}

	applyFileOps(memoryDir, parsed.Files, &result)
	applyDeleteOps(memoryDir, allowDelete, parsed.Deletes, &result)
	applyProposalOps(proposalsDir, parsed.Proposals, &result)

	if len(result.Applied) > 0 || len(result.Deleted) > 0 {
		if err := guardrails.RebuildIndex(memoryDir); err != nil {
			return applyResult{}, domain.Error(err, "rebuild memory index failed")
		}
	}

	return result, nil
}

func reject(result *applyResult, name, reason string) {
	result.Rejected = append(result.Rejected, rejectedOp{Name: name, Reason: reason})
}

func applyFileOps(memoryDir string, files []ops.FileOp, result *applyResult) {
	for _, f := range files {
		name, err := guardrails.NormalizeFilename(f.Name)
		if err != nil {
			reject(result, f.Name, err.Error())
			continue
		}
		if err := guardrails.ValidateFrontmatter(f.Content); err != nil {
			reject(result, name, err.Error())
			continue
		}
		if err := guardrails.ScanSecrets(f.Content); err != nil {
			reject(result, name, err.Error())
			continue
		}
		if err := os.WriteFile(filepath.Join(memoryDir, name), f.Content, 0o600); err != nil {
			reject(result, name, err.Error())
			continue
		}
		result.Applied = append(result.Applied, name)
	}
}

func applyDeleteOps(memoryDir string, allowDelete bool, deletes []string, result *applyResult) {
	for _, raw := range deletes {
		name, err := guardrails.NormalizeFilename(raw)
		if err != nil {
			reject(result, raw, err.Error())
			continue
		}
		if !allowDelete {
			reject(result, name, reasonDeletesNotAllowed)
			continue
		}

		path := filepath.Join(memoryDir, name)
		if _, statErr := os.Stat(path); statErr != nil {
			reject(result, name, reasonNoSuchEntry)
			continue
		}
		if err := os.Remove(path); err != nil {
			reject(result, name, err.Error())
			continue
		}
		result.Deleted = append(result.Deleted, name)
	}
}

func applyProposalOps(proposalsDir string, proposals []ops.FileOp, result *applyResult) {
	for _, p := range proposals {
		if proposalsDir == "" {
			reject(result, p.Name, reasonProposalsNotAllowed)
			continue
		}
		if !validProposalSourcePath(p.Name) {
			reject(result, p.Name, reasonInvalidProposalPath)
			continue
		}
		if err := guardrails.ScanSecrets(p.Content); err != nil {
			reject(result, p.Name, err.Error())
			continue
		}

		flattened := strings.ReplaceAll(p.Name, "/", "__")
		if err := writeProposalFiles(proposalsDir, flattened, p); err != nil {
			reject(result, p.Name, err.Error())
			continue
		}
		result.Proposals = append(result.Proposals, flattened)
	}
}

func writeProposalFiles(proposalsDir, flattened string, p ops.FileOp) error {
	if err := os.MkdirAll(proposalsDir, 0o750); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(proposalsDir, flattened), p.Content, 0o600); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(proposalsDir, flattened+".path"), []byte(p.Name), 0o600)
}

func validProposalSourcePath(p string) bool {
	if p == "" || strings.Contains(p, "..") || pathpkg.IsAbs(p) {
		return false
	}
	if pathpkg.Clean(p) != p {
		return false
	}
	return strings.HasPrefix(p, "global/")
}
