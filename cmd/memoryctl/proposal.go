package main

import (
	"context"
	"flag"
	"os"
	pathpkg "path"
	"path/filepath"
	"strings"

	"claude-memory-sync/internal/domain"
	"claude-memory-sync/internal/guardrails"
)

const (
	proposalRulesPrefix  = "global/rules/"
	proposalSkillsPrefix = "global/skills/"
	proposalAgentsPrefix = "global/agents/"
	proposalClaudeMDPath = "global/CLAUDE.md"
)

const (
	errEmptyProposalPath      cliError = "proposal path is empty"
	errProposalPathTraversal  cliError = "proposal path is unsafe"
	errProposalPathNotAllowed cliError = "proposal path is not in an allowed location"
	errProposalRequiresMD     cliError = "proposal path requires .md extension"
)

func cmdProposalApply(_ context.Context, args []string) error {
	fs := flag.NewFlagSet("proposal-apply", flag.ExitOnError)
	workDir := fs.String("work-dir", "", "work directory")
	file := fs.String("file", "", "flattened proposal file")
	if err := fs.Parse(args); err != nil {
		return domain.Error(err, "parse proposal-apply flags failed")
	}

	return applyProposal(*workDir, *file)
}

func applyProposal(workDir, flattenedFile string) error {
	realPath, err := resolveProposalRealPath(flattenedFile)
	if err != nil {
		return err
	}

	if err := validateProposalTarget(realPath); err != nil {
		return domain.Error(err, "reject proposal target")
	}

	content, err := os.ReadFile(flattenedFile)
	if err != nil {
		return domain.Error(err, "read proposal file failed")
	}

	if err := guardrails.ScanSecrets(content); err != nil {
		return domain.Error(err, "proposal secret scan failed")
	}

	dest, err := resolveProposalDest(workDir, realPath)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o750); err != nil {
		return domain.Error(err, "create proposal target directory failed")
	}
	if err := writeProposalContent(dest, content); err != nil {
		return domain.Error(err, "write proposal file failed")
	}
	return nil
}

func writeProposalContent(dest string, content []byte) error {
	file, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer func() {
		_ = file.Close()
	}()

	_, err = file.Write(content)
	return err
}

func resolveProposalDest(workDir, realPath string) (string, error) {
	base := filepath.Clean(workDir)
	dest := filepath.Clean(filepath.Join(base, filepath.FromSlash(realPath)))

	if dest != base && !strings.HasPrefix(dest, base+string(filepath.Separator)) {
		return "", domain.Error(errProposalPathTraversal, "resolved proposal path escapes work directory")
	}

	return dest, nil
}

func resolveProposalRealPath(flattenedFile string) (string, error) {
	sidecarPath := flattenedFile + ".path"

	content, err := os.ReadFile(sidecarPath)
	if err == nil {
		return strings.TrimSpace(string(content)), nil
	}
	if !os.IsNotExist(err) {
		return "", domain.Error(err, "read proposal sidecar failed")
	}

	base := filepath.Base(flattenedFile)
	return strings.ReplaceAll(base, "__", "/"), nil
}

func validateProposalTarget(p string) error {
	if p == "" {
		return errEmptyProposalPath
	}
	if strings.Contains(p, "..") || pathpkg.IsAbs(p) || pathpkg.Clean(p) != p {
		return errProposalPathTraversal
	}

	switch {
	case p == proposalClaudeMDPath:
		return nil
	case strings.HasPrefix(p, proposalRulesPrefix):
		return requireMarkdownSuffix(p)
	case strings.HasPrefix(p, proposalSkillsPrefix), strings.HasPrefix(p, proposalAgentsPrefix):
		return nil
	case isProjectMemoryProposal(p):
		return nil
	default:
		return errProposalPathNotAllowed
	}
}

func isProjectMemoryProposal(p string) bool {
	parts := strings.Split(p, "/")
	if len(parts) != 4 || parts[0] != "projects" || parts[2] != "memory" {
		return false
	}
	name := parts[3]
	return parts[1] != "" && name != memoryIndexFile && strings.HasSuffix(name, ".md")
}

func requireMarkdownSuffix(p string) error {
	if !strings.HasSuffix(p, ".md") {
		return errProposalRequiresMD
	}
	return nil
}
