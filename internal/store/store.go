package store

//go:generate moq -out store_mock.go . Store:MockStore

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"claude-memory-sync/internal/manifest"
)

const (
	canonicalDir = "canonical"
	clientsDir   = "clients"
	gitKeepFile  = ".gitkeep"
	gitUserName  = "memory-server"
	gitUserEmail = "memory-server@local"
	gitBranch    = "storage"
)

type Store interface {
	Tree(namespace string) (manifest.Manifest, error)
	Read(namespace, path string) ([]byte, error)
	Write(namespace, path string, content []byte, clientID string) error
	Delete(namespace, path string, clientID string) error
}

type gitStore struct {
	root string
}

func New(root string) (Store, error) {
	s := &gitStore{root: root}
	if err := s.init(); err != nil {
		return nil, fmt.Errorf("init store: %w", err)
	}
	return s, nil
}

func (s *gitStore) init() error {
	if err := os.MkdirAll(s.root, 0o755); err != nil {
		return fmt.Errorf("create root dir: %w", err)
	}

	isRepo := true
	if _, err := os.Stat(filepath.Join(s.root, ".git")); os.IsNotExist(err) {
		isRepo = false
	}

	if !isRepo {
		if _, err := runGit(s.root, "init", "--initial-branch="+gitBranch); err != nil {
			return fmt.Errorf("git init: %w", err)
		}
		if _, err := runGit(s.root, "config", "--local", "user.name", gitUserName); err != nil {
			return fmt.Errorf("git config user.name: %w", err)
		}
		if _, err := runGit(s.root, "config", "--local", "user.email", gitUserEmail); err != nil {
			return fmt.Errorf("git config user.email: %w", err)
		}
	}

	if err := ensureDirWithKeep(filepath.Join(s.root, canonicalDir)); err != nil {
		return fmt.Errorf("ensure canonical dir: %w", err)
	}
	if err := ensureDirWithKeep(filepath.Join(s.root, clientsDir)); err != nil {
		return fmt.Errorf("ensure clients dir: %w", err)
	}

	if _, err := runGit(s.root, "add", "-A"); err != nil {
		return fmt.Errorf("git add: %w", err)
	}
	hasChanges, err := gitHasStagedChanges(s.root)
	if err != nil {
		return fmt.Errorf("check staged changes: %w", err)
	}
	if hasChanges {
		if _, err := runGit(s.root, "commit", "-m", "init: memory-server storage"); err != nil {
			return fmt.Errorf("git commit init: %w", err)
		}
	}

	slog.Info("store initialized", slog.String("root", s.root))
	return nil
}

func ensureDirWithKeep(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}
	keep := filepath.Join(dir, gitKeepFile)
	if _, err := os.Stat(keep); os.IsNotExist(err) {
		if err := os.WriteFile(keep, nil, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", keep, err)
		}
	}
	return nil
}

func validatePath(path string) error {
	if path == "" {
		return fmt.Errorf("%w: empty path", ErrInvalidPath)
	}
	if strings.HasPrefix(path, "/") {
		return fmt.Errorf("%w: %s", ErrInvalidPath, path)
	}
	if strings.Contains(path, "..") {
		return fmt.Errorf("%w: %s", ErrInvalidPath, path)
	}
	return nil
}

func validateNamespace(namespace string) error {
	if namespace == "" {
		return fmt.Errorf("%w: empty namespace", ErrInvalidNamespace)
	}
	if strings.Contains(namespace, "..") {
		return fmt.Errorf("%w: %s", ErrInvalidNamespace, namespace)
	}
	if namespace == canonicalDir {
		return nil
	}
	if strings.HasPrefix(namespace, clientsDir+"/") && len(namespace) > len(clientsDir)+1 {
		return nil
	}
	return fmt.Errorf("%w: %s", ErrInvalidNamespace, namespace)
}

func (s *gitStore) namespaceDir(namespace string) (string, error) {
	if err := validateNamespace(namespace); err != nil {
		return "", err
	}
	return filepath.Join(s.root, filepath.FromSlash(namespace)), nil
}

func (s *gitStore) resolve(namespace, path string) (string, string, error) {
	if err := validatePath(path); err != nil {
		return "", "", err
	}
	nsDir, err := s.namespaceDir(namespace)
	if err != nil {
		return "", "", err
	}
	relPath := filepath.Join(namespace, filepath.FromSlash(path))
	return filepath.Join(nsDir, filepath.FromSlash(path)), relPath, nil
}

func (s *gitStore) Tree(namespace string) (manifest.Manifest, error) {
	nsDir, err := s.namespaceDir(namespace)
	if err != nil {
		return nil, err
	}

	whitelist := func(relPath string, d os.DirEntry) bool {
		return !strings.HasPrefix(d.Name(), ".")
	}

	m, err := manifest.Build(nsDir, whitelist)
	if err != nil {
		return nil, fmt.Errorf("build tree for %s: %w", namespace, err)
	}
	return m, nil
}

func (s *gitStore) Read(namespace, path string) ([]byte, error) {
	target, _, err := s.resolve(namespace, path)
	if err != nil {
		return nil, err
	}

	content, err := os.ReadFile(target)
	if os.IsNotExist(err) {
		return nil, fmt.Errorf("%w: %s/%s", ErrNotFound, namespace, path)
	}
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}
	return content, nil
}

func (s *gitStore) Write(namespace, path string, content []byte, clientID string) error {
	target, relPath, err := s.resolve(namespace, path)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fmt.Errorf("create parent dir: %w", err)
	}
	if err := os.WriteFile(target, content, 0o644); err != nil {
		return fmt.Errorf("write file: %w", err)
	}

	if err := s.commit(relPath, clientID, namespace, path); err != nil {
		return err
	}

	slog.Info("wrote file",
		slog.String("namespace", namespace),
		slog.String("path", path),
		slog.String("clientId", clientID),
	)
	return nil
}

func (s *gitStore) Delete(namespace, path string, clientID string) error {
	target, relPath, err := s.resolve(namespace, path)
	if err != nil {
		return err
	}

	if _, err := os.Stat(target); os.IsNotExist(err) {
		return nil
	}

	if _, err := runGit(s.root, "rm", "-f", "--", relPath); err != nil {
		return fmt.Errorf("git rm: %w", err)
	}

	hasChanges, err := gitHasStagedChanges(s.root)
	if err != nil {
		return fmt.Errorf("check staged changes: %w", err)
	}
	if hasChanges {
		msg := fmt.Sprintf("sync: %s %s/%s", clientID, namespace, path)
		if _, err := runGit(s.root, "commit", "-m", msg); err != nil {
			return fmt.Errorf("git commit: %w", err)
		}
	}

	slog.Info("deleted file",
		slog.String("namespace", namespace),
		slog.String("path", path),
		slog.String("clientId", clientID),
	)
	return nil
}

func (s *gitStore) commit(relPath, clientID, namespace, path string) error {
	if _, err := runGit(s.root, "add", "--", relPath); err != nil {
		return fmt.Errorf("git add: %w", err)
	}

	hasChanges, err := gitHasStagedChanges(s.root)
	if err != nil {
		return fmt.Errorf("check staged changes: %w", err)
	}
	if !hasChanges {
		return nil
	}

	msg := fmt.Sprintf("sync: %s %s/%s", clientID, namespace, path)
	if _, err := runGit(s.root, "commit", "-m", msg); err != nil {
		return fmt.Errorf("git commit: %w", err)
	}
	return nil
}
