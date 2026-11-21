package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"claude-memory-sync/internal/domain"
)

func TestLoadConfig(t *testing.T) {
	t.Run("defaults apply when only required vars are set", func(t *testing.T) {
		t.Setenv("PIPELINE_URL", "http://pipeline-api:8090")
		t.Setenv("PIPELINE_TOKEN", "admin")

		cfg, err := loadConfig()

		require.NoError(t, err)
		assert.Equal(t, config{
			addr:     ":8082",
			webDir:   "/srv/web",
			pipeline: "http://pipeline-api:8090",
			token:    "admin",
			reposDir: "/data/repos",
			workDir:  "/data/work/canonical",
			lock:     "/data/state/canonical.lock",
		}, cfg)
	})

	t.Run("overrides are read", func(t *testing.T) {
		t.Setenv("PIPELINE_URL", "http://p")
		t.Setenv("PIPELINE_TOKEN", "admin")
		t.Setenv("SYNTH_ADDR", ":9000")
		t.Setenv("SYNTH_WEB_DIR", "/web")
		t.Setenv("MEMORY_REPOS_DIR", "/r")
		t.Setenv("MEMORY_WORK_DIR", "/w")
		t.Setenv("MEMORY_LOCK", "/l")
		t.Setenv("CLAUDE_CODE_OAUTH_TOKEN", "bootstrap")

		cfg, err := loadConfig()

		require.NoError(t, err)
		assert.Equal(t, config{addr: ":9000", webDir: "/web", pipeline: "http://p", token: "admin", reposDir: "/r", workDir: "/w", lock: "/l", oauthToken: "bootstrap"}, cfg)
	})

	t.Run("missing pipeline url is a validation error", func(t *testing.T) {
		t.Setenv("PIPELINE_URL", "")
		t.Setenv("PIPELINE_TOKEN", "admin")

		_, err := loadConfig()

		require.ErrorIs(t, err, domain.ErrValidation)
	})

	t.Run("missing pipeline token is a validation error", func(t *testing.T) {
		t.Setenv("PIPELINE_URL", "http://p")
		t.Setenv("PIPELINE_TOKEN", "")

		_, err := loadConfig()

		require.ErrorIs(t, err, domain.ErrValidation)
	})
}
