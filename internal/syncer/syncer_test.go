package syncer

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Orfeo42/claude-memory-sync/internal/manifest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAgentUpSync(t *testing.T) {
	t.Run("PUTs new local file then DELETEs it once removed", func(t *testing.T) {
		stateDir := t.TempDir()
		fileDir := t.TempDir()
		filePath := filepath.Join(fileDir, "CLAUDE.md")
		require.NoError(t, os.WriteFile(filePath, []byte("hello"), 0o644))

		var putPath string
		var putContent []byte
		var deletePath string
		mock := &MockHTTPClient{
			PutClientFileFunc: func(_ context.Context, path string, content []byte) error {
				putPath, putContent = path, content
				return nil
			},
			DeleteClientFileFunc: func(_ context.Context, path string) error {
				deletePath = path
				return nil
			},
		}

		agent := New(Config{StateDir: stateDir, ClientID: "host-a"}, mock)

		local := manifest.Manifest{{Path: "global/CLAUDE.md", SHA256: "h1", Size: 5}}
		localPaths := map[string]string{"global/CLAUDE.md": filePath}

		require.NoError(t, agent.upSync(context.Background(), local, localPaths))
		assert.Equal(t, "global/CLAUDE.md", putPath)
		assert.Equal(t, "hello", string(putContent))
		assert.Empty(t, deletePath)

		require.NoError(t, agent.upSync(context.Background(), manifest.Manifest{}, map[string]string{}))
		assert.Equal(t, "global/CLAUDE.md", deletePath)
	})

	t.Run("makes no calls when local matches the saved base", func(t *testing.T) {
		stateDir := t.TempDir()
		mock := &MockHTTPClient{
			PutClientFileFunc: func(context.Context, string, []byte) error {
				t.Fatal("unexpected PUT call")
				return nil
			},
			DeleteClientFileFunc: func(context.Context, string) error {
				t.Fatal("unexpected DELETE call")
				return nil
			},
		}
		agent := New(Config{StateDir: stateDir, ClientID: "host-a"}, mock)

		local := manifest.Manifest{{Path: "global/CLAUDE.md", SHA256: "h1", Size: 5}}
		require.NoError(t, saveManifest(agent.clientBasePath(), local))

		require.NoError(t, agent.upSync(context.Background(), local, map[string]string{}))
	})

	t.Run("saves local manifest as the new base", func(t *testing.T) {
		stateDir := t.TempDir()
		mock := &MockHTTPClient{
			PutClientFileFunc: func(context.Context, string, []byte) error { return nil },
		}
		agent := New(Config{StateDir: stateDir, ClientID: "host-a"}, mock)

		fileDir := t.TempDir()
		filePath := filepath.Join(fileDir, "CLAUDE.md")
		require.NoError(t, os.WriteFile(filePath, []byte("hello"), 0o644))
		local := manifest.Manifest{{Path: "global/CLAUDE.md", SHA256: "h1", Size: 5}}

		require.NoError(t, agent.upSync(context.Background(), local, map[string]string{"global/CLAUDE.md": filePath}))

		saved, err := loadManifest(agent.clientBasePath())
		require.NoError(t, err)
		assert.Equal(t, local, saved)
	})
}

func TestAgentDownSync(t *testing.T) {
	t.Run("applies canonical changed file when local unchanged", func(t *testing.T) {
		claudeDir := t.TempDir()
		stateDir := t.TempDir()

		mock := &MockHTTPClient{
			CanonicalTreeFunc: func(context.Context) (manifest.Manifest, error) {
				return manifest.Manifest{{Path: "global/CLAUDE.md", SHA256: "h2", Size: 3}}, nil
			},
			GetCanonicalFileFunc: func(_ context.Context, path string) ([]byte, error) {
				require.Equal(t, "global/CLAUDE.md", path)
				return []byte("new"), nil
			},
		}
		agent := New(Config{ClaudeDir: claudeDir, StateDir: stateDir, SlugPrefix: "-home-orfeo42"}, mock)

		require.NoError(t, agent.downSync(context.Background(), manifest.Manifest{}))

		content, err := os.ReadFile(filepath.Join(claudeDir, "CLAUDE.md"))
		require.NoError(t, err)
		assert.Equal(t, "new", string(content))
	})

	t.Run("deletes local file when canonical deleted and local unchanged", func(t *testing.T) {
		claudeDir := t.TempDir()
		stateDir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(claudeDir, "CLAUDE.md"), []byte("old"), 0o644))

		canonicalBase := manifest.Manifest{{Path: "global/CLAUDE.md", SHA256: "h1", Size: 3}}
		require.NoError(t, saveManifest(filepath.Join(stateDir, canonicalBaseFile), canonicalBase))

		mock := &MockHTTPClient{
			CanonicalTreeFunc: func(context.Context) (manifest.Manifest, error) {
				return manifest.Manifest{}, nil
			},
		}
		agent := New(Config{ClaudeDir: claudeDir, StateDir: stateDir, SlugPrefix: "-home-orfeo42"}, mock)

		local := manifest.Manifest{{Path: "global/CLAUDE.md", SHA256: "h1", Size: 3}}
		require.NoError(t, agent.downSync(context.Background(), local))

		_, statErr := os.Stat(filepath.Join(claudeDir, "CLAUDE.md"))
		assert.True(t, os.IsNotExist(statErr))
	})

	t.Run("local wins: skips update when both local and canonical changed", func(t *testing.T) {
		claudeDir := t.TempDir()
		stateDir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(claudeDir, "CLAUDE.md"), []byte("local-edit"), 0o644))

		canonicalBase := manifest.Manifest{{Path: "global/CLAUDE.md", SHA256: "base", Size: 3}}
		require.NoError(t, saveManifest(filepath.Join(stateDir, canonicalBaseFile), canonicalBase))

		mock := &MockHTTPClient{
			CanonicalTreeFunc: func(context.Context) (manifest.Manifest, error) {
				return manifest.Manifest{{Path: "global/CLAUDE.md", SHA256: "remote-new", Size: 3}}, nil
			},
			GetCanonicalFileFunc: func(context.Context, string) ([]byte, error) {
				t.Fatal("should not fetch canonical file when local wins")
				return nil, nil
			},
		}
		agent := New(Config{ClaudeDir: claudeDir, StateDir: stateDir, SlugPrefix: "-home-orfeo42"}, mock)

		local := manifest.Manifest{{Path: "global/CLAUDE.md", SHA256: "local-new", Size: 3}}
		require.NoError(t, agent.downSync(context.Background(), local))

		content, err := os.ReadFile(filepath.Join(claudeDir, "CLAUDE.md"))
		require.NoError(t, err)
		assert.Equal(t, "local-edit", string(content))
	})

	t.Run("deletion vs never-had: canonical file absent that was never synced causes no action", func(t *testing.T) {
		claudeDir := t.TempDir()
		stateDir := t.TempDir()

		mock := &MockHTTPClient{
			CanonicalTreeFunc: func(context.Context) (manifest.Manifest, error) {
				return manifest.Manifest{}, nil
			},
		}
		agent := New(Config{ClaudeDir: claudeDir, StateDir: stateDir, SlugPrefix: "-home-orfeo42"}, mock)

		require.NoError(t, agent.downSync(context.Background(), manifest.Manifest{}))

		entries, err := os.ReadDir(claudeDir)
		require.NoError(t, err)
		assert.Empty(t, entries)
	})

	t.Run("saves canonical tree as the new base", func(t *testing.T) {
		claudeDir := t.TempDir()
		stateDir := t.TempDir()

		canonicalCurrent := manifest.Manifest{{Path: "global/CLAUDE.md", SHA256: "h2", Size: 3}}
		mock := &MockHTTPClient{
			CanonicalTreeFunc: func(context.Context) (manifest.Manifest, error) {
				return canonicalCurrent, nil
			},
			GetCanonicalFileFunc: func(context.Context, string) ([]byte, error) {
				return []byte("new"), nil
			},
		}
		agent := New(Config{ClaudeDir: claudeDir, StateDir: stateDir, SlugPrefix: "-home-orfeo42"}, mock)

		require.NoError(t, agent.downSync(context.Background(), manifest.Manifest{}))

		saved, err := loadManifest(agent.canonicalBasePath())
		require.NoError(t, err)
		assert.Equal(t, canonicalCurrent, saved)
	})
}

func TestAgentRunCycle(t *testing.T) {
	t.Run("runs up-sync before down-sync each cycle", func(t *testing.T) {
		claudeDir := t.TempDir()
		stateDir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(claudeDir, "CLAUDE.md"), []byte("local content"), 0o644))

		var order []string
		mock := &MockHTTPClient{
			PutClientFileFunc: func(context.Context, string, []byte) error {
				order = append(order, "put")
				return nil
			},
			DeleteClientFileFunc: func(context.Context, string) error {
				order = append(order, "delete")
				return nil
			},
			CanonicalTreeFunc: func(context.Context) (manifest.Manifest, error) {
				order = append(order, "canonicalTree")
				return manifest.Manifest{}, nil
			},
			GetCanonicalFileFunc: func(context.Context, string) ([]byte, error) {
				order = append(order, "getCanonical")
				return nil, nil
			},
		}

		agent := New(Config{ClaudeDir: claudeDir, StateDir: stateDir, SlugPrefix: "-home-orfeo42", ClientID: "host-a"}, mock)

		require.NoError(t, agent.RunCycle(context.Background()))

		require.Len(t, order, 2)
		assert.Equal(t, "put", order[0])
		assert.Equal(t, "canonicalTree", order[1])
	})

	t.Run("propagates scan errors", func(t *testing.T) {
		claudeDir := filepath.Join(t.TempDir(), "CLAUDE.md")
		require.NoError(t, os.WriteFile(claudeDir, []byte("not a dir"), 0o644))
		stateDir := t.TempDir()

		mock := &MockHTTPClient{}
		agent := New(Config{ClaudeDir: claudeDir, StateDir: stateDir, SlugPrefix: "-home-orfeo42", ClientID: "host-a"}, mock)

		err := agent.RunCycle(context.Background())
		require.Error(t, err)
	})
}
