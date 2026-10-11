package client

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"claude-memory-sync/internal/domain"
)

func TestConfigFromEnv(t *testing.T) {
	t.Run("both vars set", func(t *testing.T) {
		t.Setenv("PIPELINE_URL", "http://pipeline:8090")
		t.Setenv("PIPELINE_TOKEN", "secret")

		cfg, err := ConfigFromEnv()
		require.NoError(t, err)
		assert.Equal(t, Config{URL: "http://pipeline:8090", Token: "secret"}, cfg)
	})

	t.Run("missing url", func(t *testing.T) {
		t.Setenv("PIPELINE_URL", "")
		t.Setenv("PIPELINE_TOKEN", "secret")

		_, err := ConfigFromEnv()
		require.ErrorIs(t, err, domain.ErrValidation)
	})

	t.Run("missing token", func(t *testing.T) {
		t.Setenv("PIPELINE_URL", "http://pipeline:8090")
		t.Setenv("PIPELINE_TOKEN", "")

		_, err := ConfigFromEnv()
		require.ErrorIs(t, err, domain.ErrValidation)
	})
}
