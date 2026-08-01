package syncer

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnsureCanonicalMirror(t *testing.T) {
	t.Run("succeeds against an empty canonical repo with no commits", func(t *testing.T) {
		cfg, _ := newTestConfig(t, "host-a", "-home-orfeo42")

		require.NoError(t, ensureCanonicalMirror(context.Background(), cfg))

		assert.DirExists(t, filepath.Join(mirrorDir(cfg), ".git"))
	})

	t.Run("clones and checks out canonical content", func(t *testing.T) {
		cfg, base := newTestConfig(t, "host-a", "-home-orfeo42")
		seedBareFile(t, canonicalBareDir(base), "global/rules/other.md", "rule content")

		require.NoError(t, ensureCanonicalMirror(context.Background(), cfg))

		content, err := os.ReadFile(filepath.Join(mirrorDir(cfg), "global", "rules", "other.md"))
		require.NoError(t, err)
		assert.Equal(t, "rule content", string(content))
	})

	t.Run("resets to latest canonical content on repeated calls", func(t *testing.T) {
		cfg, base := newTestConfig(t, "host-a", "-home-orfeo42")
		seedBareFile(t, canonicalBareDir(base), "global/rules/example.md", "first")

		require.NoError(t, ensureCanonicalMirror(context.Background(), cfg))

		seedBareFile(t, canonicalBareDir(base), "global/rules/example.md", "second")
		require.NoError(t, ensureCanonicalMirror(context.Background(), cfg))

		content, err := os.ReadFile(filepath.Join(mirrorDir(cfg), "global", "rules", "example.md"))
		require.NoError(t, err)
		assert.Equal(t, "second", string(content))
	})
}

func TestCanonicalManifest(t *testing.T) {
	t.Run("builds a manifest of mirrored files excluding .git", func(t *testing.T) {
		cfg, base := newTestConfig(t, "host-a", "-home-orfeo42")
		seedBareFile(t, canonicalBareDir(base), "global/rules/example.md", "rule content")
		require.NoError(t, ensureCanonicalMirror(context.Background(), cfg))

		m, err := canonicalManifest(cfg)
		require.NoError(t, err)
		require.Len(t, m, 1)
		assert.Equal(t, "global/rules/example.md", m[0].Path)
	})

	t.Run("returns empty manifest when mirror has no content yet", func(t *testing.T) {
		cfg, _ := newTestConfig(t, "host-b", "-home-orfeo42")
		require.NoError(t, ensureCanonicalMirror(context.Background(), cfg))

		m, err := canonicalManifest(cfg)
		require.NoError(t, err)
		assert.Empty(t, m)
	})
}
