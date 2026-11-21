package settings

import (
	"strings"
	"testing"

	"claude-memory-sync/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseKey(t *testing.T) {
	t.Run("accepts each namespace", func(t *testing.T) {
		for _, raw := range []string{"prompt.intake", "param.max-items", "schedule.daily_synth", "secret.token1"} {
			key, err := ParseKey(raw)
			require.NoError(t, err, raw)
			assert.Equal(t, Key(raw), key)
		}
	})

	t.Run("rejects unknown namespace", func(t *testing.T) {
		_, err := ParseKey("other.name")
		require.ErrorIs(t, err, domain.ErrValidation)
	})

	t.Run("rejects empty name", func(t *testing.T) {
		_, err := ParseKey("prompt.")
		require.ErrorIs(t, err, domain.ErrValidation)
	})

	t.Run("rejects uppercase and leading dash", func(t *testing.T) {
		_, err := ParseKey("prompt.Intake")
		require.ErrorIs(t, err, domain.ErrValidation)
		_, err = ParseKey("prompt.-x")
		require.ErrorIs(t, err, domain.ErrValidation)
	})

	t.Run("rejects name longer than 64", func(t *testing.T) {
		_, err := ParseKey("prompt." + strings.Repeat("a", 65))
		require.ErrorIs(t, err, domain.ErrValidation)
	})

	t.Run("accepts name of 64", func(t *testing.T) {
		_, err := ParseKey("prompt." + strings.Repeat("a", 64))
		require.NoError(t, err)
	})
}

func TestKeyIsSecret(t *testing.T) {
	t.Run("secret prefix is secret", func(t *testing.T) {
		assert.True(t, Key("secret.token").IsSecret())
	})

	t.Run("other prefixes are not secret", func(t *testing.T) {
		assert.False(t, Key("prompt.token").IsSecret())
	})
}
