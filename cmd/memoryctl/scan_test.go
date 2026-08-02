package main

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScanClients(t *testing.T) {
	t.Run("marker unset classifies config and memory candidates", func(t *testing.T) {
		reposDir := filepath.Join(t.TempDir(), "repos")
		clientRepo := filepath.Join(reposDir, "clients", "client-a.git")
		initBareRepo(t, clientRepo)

		when := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
		head := commitFiles(t, clientRepo, map[string]string{
			"global/CLAUDE.md":                "hello",
			"projects/proj1/memory/foo.md":    "foo content",
			"projects/proj1/memory/MEMORY.md": "index",
		}, "seed", when)

		result, err := scanClients(context.Background(), reposDir)
		require.NoError(t, err)
		require.Len(t, result.Clients, 1)

		client := result.Clients[0]
		assert.Equal(t, "client-a", client.ID)
		assert.Equal(t, head, client.Head)
		assert.Empty(t, client.Marker)
		require.Len(t, client.Config, 1)
		assert.Equal(t, "global/CLAUDE.md", client.Config[0].Path)
		assert.False(t, client.Config[0].Deleted)
		assert.Positive(t, client.Config[0].Timestamp)
		require.Len(t, client.Memory, 1)
		assert.Equal(t, "proj1", client.Memory[0].Project)
		assert.Equal(t, "projects/proj1/memory/foo.md", client.Memory[0].Path)
	})

	t.Run("marker set filters diff to changes after marker", func(t *testing.T) {
		reposDir := filepath.Join(t.TempDir(), "repos")
		clientRepo := filepath.Join(reposDir, "clients", "client-b.git")
		initBareRepo(t, clientRepo)

		firstHead := commitFiles(t, clientRepo, map[string]string{
			"global/CLAUDE.md": "v1",
		}, "seed", time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC))
		runGit(t, "", nil, "-C", clientRepo, "update-ref", "refs/intake/last-processed", firstHead)

		secondHead := commitFiles(t, clientRepo, map[string]string{
			"projects/proj1/memory/bar.md": "bar content",
		}, "second", time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC))

		result, err := scanClients(context.Background(), reposDir)
		require.NoError(t, err)
		require.Len(t, result.Clients, 1)

		client := result.Clients[0]
		assert.Equal(t, secondHead, client.Head)
		assert.Equal(t, firstHead, client.Marker)
		assert.Empty(t, client.Config)
		require.Len(t, client.Memory, 1)
		assert.Equal(t, "projects/proj1/memory/bar.md", client.Memory[0].Path)
	})

	t.Run("marker equals head returns empty candidates", func(t *testing.T) {
		reposDir := filepath.Join(t.TempDir(), "repos")
		clientRepo := filepath.Join(reposDir, "clients", "client-c.git")
		initBareRepo(t, clientRepo)

		head := commitFiles(t, clientRepo, map[string]string{
			"global/CLAUDE.md": "v1",
		}, "seed", time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC))
		runGit(t, "", nil, "-C", clientRepo, "update-ref", "refs/intake/last-processed", head)

		result, err := scanClients(context.Background(), reposDir)
		require.NoError(t, err)
		require.Len(t, result.Clients, 1)

		client := result.Clients[0]
		assert.Equal(t, head, client.Marker)
		assert.Empty(t, client.Config)
		assert.Empty(t, client.Memory)
	})

	t.Run("delete status marks deleted config candidate", func(t *testing.T) {
		reposDir := filepath.Join(t.TempDir(), "repos")
		clientRepo := filepath.Join(reposDir, "clients", "client-d.git")
		initBareRepo(t, clientRepo)

		firstHead := commitFiles(t, clientRepo, map[string]string{
			"global/CLAUDE.md": "v1",
		}, "seed", time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC))
		runGit(t, "", nil, "-C", clientRepo, "update-ref", "refs/intake/last-processed", firstHead)

		removeFiles(t, clientRepo, []string{"global/CLAUDE.md"}, "remove", time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC))

		result, err := scanClients(context.Background(), reposDir)
		require.NoError(t, err)
		require.Len(t, result.Clients, 1)

		client := result.Clients[0]
		require.Len(t, client.Config, 1)
		assert.True(t, client.Config[0].Deleted)
	})
}
