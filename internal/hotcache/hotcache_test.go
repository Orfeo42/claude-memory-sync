package hotcache

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWrite(t *testing.T) {
	t.Run("writes digest into hot block", func(t *testing.T) {
		memoryDir := t.TempDir()

		result, err := Write(memoryDir, 1024, strings.NewReader("some digest content"))
		require.NoError(t, err)
		assert.True(t, result.Written)
		assert.Empty(t, result.Reason)
	})

	t.Run("digest too large is reported without error", func(t *testing.T) {
		memoryDir := t.TempDir()

		result, err := Write(memoryDir, 4, strings.NewReader("way too long for the limit"))
		require.NoError(t, err)
		assert.False(t, result.Written)
		assert.Equal(t, "digest too large", result.Reason)
	})

	t.Run("empty digest is reported without error", func(t *testing.T) {
		memoryDir := t.TempDir()

		result, err := Write(memoryDir, 1024, strings.NewReader("   \n  "))
		require.NoError(t, err)
		assert.False(t, result.Written)
		assert.Equal(t, "empty digest", result.Reason)
	})
}

func TestExtractDigest(t *testing.T) {
	t.Run("returns content between hot markers", func(t *testing.T) {
		content := "# Index\n\n<!-- HOT:BEGIN -->\nline one\nline two\n<!-- HOT:END -->\n"

		assert.Equal(t, "line one\nline two", ExtractDigest(content))
	})

	t.Run("missing markers return empty", func(t *testing.T) {
		assert.Empty(t, ExtractDigest("# Index\n"))
	})

	t.Run("end before begin returns empty", func(t *testing.T) {
		assert.Empty(t, ExtractDigest("<!-- HOT:END -->\nx\n<!-- HOT:BEGIN -->\n"))
	})
}
