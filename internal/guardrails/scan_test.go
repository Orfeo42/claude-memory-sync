package guardrails_test

import (
	"strings"
	"testing"

	"claude-memory-sync/internal/guardrails"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScanSecrets(t *testing.T) {
	t.Run("detects a secret-shaped token", func(t *testing.T) {
		fakeToken := "gh" + "p_" + strings.Repeat("a", 24)
		content := []byte("some notes\ntoken here: " + fakeToken + "\n")

		err := guardrails.ScanSecrets(content)
		require.Error(t, err)
		assert.ErrorIs(t, err, guardrails.ErrSecretDetected)
	})

	t.Run("passes clean content", func(t *testing.T) {
		content := []byte("just some regular notes about the project\n")

		err := guardrails.ScanSecrets(content)
		require.NoError(t, err)
	})
}

func TestScanHidden(t *testing.T) {
	t.Run("detects a bidi override with offset and code point", func(t *testing.T) {
		err := guardrails.ScanHidden([]byte("ab\u202Ecd"))
		require.Error(t, err)
		require.ErrorIs(t, err, guardrails.ErrHiddenCharacter)
		assert.Contains(t, err.Error(), "U+202E")
		assert.Contains(t, err.Error(), "byte 2")
	})

	t.Run("passes clean content", func(t *testing.T) {
		err := guardrails.ScanHidden([]byte("just some regular notes\n"))
		require.NoError(t, err)
	})
}
