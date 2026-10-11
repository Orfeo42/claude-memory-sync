package main

import (
	"testing"

	"claude-memory-sync/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setEnv(t *testing.T, dbURL, token, adminToken, port string) {
	t.Helper()
	t.Setenv("PIPELINE_DATABASE_URL", dbURL)
	t.Setenv("PIPELINE_TOKEN", token)
	t.Setenv("PIPELINE_ADMIN_TOKEN", adminToken)
	t.Setenv("PIPELINE_PORT", port)
}

func TestLoadConfig(t *testing.T) {
	t.Run("loads all values", func(t *testing.T) {
		setEnv(t, "postgres://x", "w", "a", "9000")

		cfg, err := loadConfig()

		require.NoError(t, err)
		assert.Equal(t, config{databaseURL: "postgres://x", token: "w", adminToken: "a", port: "9000"}, cfg)
	})

	t.Run("defaults port to 8090", func(t *testing.T) {
		setEnv(t, "postgres://x", "w", "a", "")

		cfg, err := loadConfig()

		require.NoError(t, err)
		assert.Equal(t, "8090", cfg.port)
	})

	t.Run("missing database url", func(t *testing.T) {
		setEnv(t, "", "w", "a", "")

		_, err := loadConfig()

		require.ErrorIs(t, err, domain.ErrValidation)
		assert.Contains(t, domain.ErrorAttrs(err)[0].Value.String(), "PIPELINE_DATABASE_URL")
	})

	t.Run("missing token", func(t *testing.T) {
		setEnv(t, "postgres://x", "", "a", "")

		_, err := loadConfig()

		require.ErrorIs(t, err, domain.ErrValidation)
		assert.Contains(t, domain.ErrorAttrs(err)[0].Value.String(), "PIPELINE_TOKEN")
	})

	t.Run("missing admin token", func(t *testing.T) {
		setEnv(t, "postgres://x", "w", "", "")

		_, err := loadConfig()

		require.ErrorIs(t, err, domain.ErrValidation)
		assert.Contains(t, domain.ErrorAttrs(err)[0].Value.String(), "PIPELINE_ADMIN_TOKEN")
	})

	t.Run("admin token equal to writer token", func(t *testing.T) {
		setEnv(t, "postgres://x", "same", "same", "")

		_, err := loadConfig()

		require.ErrorIs(t, err, domain.ErrValidation)
	})
}
