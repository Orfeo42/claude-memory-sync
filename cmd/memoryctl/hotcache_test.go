package main

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteHotCache(t *testing.T) {
	t.Run("writes digest into hot block", func(t *testing.T) {
		memoryDir := t.TempDir()

		result, err := writeHotCache(memoryDir, 1024, strings.NewReader("some digest content"))
		require.NoError(t, err)
		assert.True(t, result.Written)
		assert.Empty(t, result.Reason)
	})

	t.Run("digest too large is reported without error", func(t *testing.T) {
		memoryDir := t.TempDir()

		result, err := writeHotCache(memoryDir, 4, strings.NewReader("way too long for the limit"))
		require.NoError(t, err)
		assert.False(t, result.Written)
		assert.Equal(t, "digest too large", result.Reason)
	})

	t.Run("empty digest is reported without error", func(t *testing.T) {
		memoryDir := t.TempDir()

		result, err := writeHotCache(memoryDir, 1024, strings.NewReader("   \n  "))
		require.NoError(t, err)
		assert.False(t, result.Written)
		assert.Equal(t, "empty digest", result.Reason)
	})
}
