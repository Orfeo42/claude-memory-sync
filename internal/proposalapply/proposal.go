package proposalapply

import (
	"os"
	"path/filepath"
	"strings"

	"claude-memory-sync/internal/domain"
	"claude-memory-sync/internal/guardrails"
	"claude-memory-sync/internal/pipeline"
)

func ApplyFile(workDir, flattenedFile string) error {
	realPath, err := resolveProposalRealPath(flattenedFile)
	if err != nil {
		return err
	}

	if err := pipeline.ValidateTarget(realPath); err != nil {
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
