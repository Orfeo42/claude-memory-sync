package manifest_test

import (
	"testing"

	"github.com/Orfeo42/claude-memory-sync/internal/manifest"
	"github.com/stretchr/testify/assert"
)

func entry(path, sha string) manifest.Entry {
	return manifest.Entry{Path: path, SHA256: sha, Size: int64(len(sha))}
}

func TestDiff(t *testing.T) {
	t.Run("unchanged when all three match", func(t *testing.T) {
		local := manifest.Manifest{entry("a.md", "h1")}
		base := manifest.Manifest{entry("a.md", "h1")}
		remote := manifest.Manifest{entry("a.md", "h1")}

		got := manifest.Diff(local, base, remote)
		assert.Equal(t, manifest.Unchanged, got["a.md"])
	})

	t.Run("localOnlyChange when local differs from base and remote matches base", func(t *testing.T) {
		local := manifest.Manifest{entry("a.md", "h2")}
		base := manifest.Manifest{entry("a.md", "h1")}
		remote := manifest.Manifest{entry("a.md", "h1")}

		got := manifest.Diff(local, base, remote)
		assert.Equal(t, manifest.LocalOnlyChange, got["a.md"])
	})

	t.Run("remoteOnlyChange when remote differs from base and local matches base", func(t *testing.T) {
		local := manifest.Manifest{entry("a.md", "h1")}
		base := manifest.Manifest{entry("a.md", "h1")}
		remote := manifest.Manifest{entry("a.md", "h2")}

		got := manifest.Diff(local, base, remote)
		assert.Equal(t, manifest.RemoteOnlyChange, got["a.md"])
	})

	t.Run("bothChanged when local and remote diverge from base", func(t *testing.T) {
		local := manifest.Manifest{entry("a.md", "h2")}
		base := manifest.Manifest{entry("a.md", "h1")}
		remote := manifest.Manifest{entry("a.md", "h3")}

		got := manifest.Diff(local, base, remote)
		assert.Equal(t, manifest.BothChanged, got["a.md"])
	})

	t.Run("localDelete when local removed a file remote still has unchanged", func(t *testing.T) {
		local := manifest.Manifest{}
		base := manifest.Manifest{entry("a.md", "h1")}
		remote := manifest.Manifest{entry("a.md", "h1")}

		got := manifest.Diff(local, base, remote)
		assert.Equal(t, manifest.LocalDelete, got["a.md"])
	})

	t.Run("remoteDelete when remote removed a file local still has unchanged", func(t *testing.T) {
		local := manifest.Manifest{entry("a.md", "h1")}
		base := manifest.Manifest{entry("a.md", "h1")}
		remote := manifest.Manifest{}

		got := manifest.Diff(local, base, remote)
		assert.Equal(t, manifest.RemoteDelete, got["a.md"])
	})

	t.Run("bothChanged when both delete and diverge", func(t *testing.T) {
		local := manifest.Manifest{entry("a.md", "h2")}
		base := manifest.Manifest{entry("a.md", "h1")}
		remote := manifest.Manifest{}

		got := manifest.Diff(local, base, remote)
		assert.Equal(t, manifest.BothChanged, got["a.md"])
	})

	t.Run("path absent everywhere but base produces no residual classification issue", func(t *testing.T) {
		local := manifest.Manifest{entry("a.md", "h1"), entry("b.md", "hb")}
		base := manifest.Manifest{entry("a.md", "h1")}
		remote := manifest.Manifest{entry("a.md", "h1")}

		got := manifest.Diff(local, base, remote)
		assert.Equal(t, manifest.LocalOnlyChange, got["b.md"])
		assert.Len(t, got, 2)
	})
}
