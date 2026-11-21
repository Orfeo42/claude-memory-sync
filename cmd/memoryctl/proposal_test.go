package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"claude-memory-sync/internal/domain"
)

func setPipelineEnv(t *testing.T, handler http.HandlerFunc) {
	t.Helper()

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	t.Setenv("PIPELINE_URL", srv.URL)
	t.Setenv("PIPELINE_TOKEN", "tok")
}

func TestCmdProposal(t *testing.T) {
	t.Run("missing subcommand is rejected", func(t *testing.T) {
		err := cmdProposal(t.Context(), nil)
		require.ErrorIs(t, err, errUnknownSubcommand)
	})

	t.Run("unknown subcommand is rejected", func(t *testing.T) {
		err := cmdProposal(t.Context(), []string{"nope"})
		require.ErrorIs(t, err, errUnknownSubcommand)
	})

	t.Run("missing env fails with validation error", func(t *testing.T) {
		t.Setenv("PIPELINE_URL", "")
		t.Setenv("PIPELINE_TOKEN", "")

		err := cmdProposal(t.Context(), []string{"get", "--id", "p1"})
		require.ErrorIs(t, err, domain.ErrValidation)
	})

	t.Run("unknown status filter is rejected", func(t *testing.T) {
		err := cmdProposal(t.Context(), []string{"list", "--status", "bogus"})
		require.ErrorIs(t, err, domain.ErrValidation)
	})
}

func TestCmdProposalImportRejections(t *testing.T) {
	t.Run("imports names through the pipeline api", func(t *testing.T) {
		posted := 0
		setPipelineEnv(t, func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/v1/rejections" && r.Method == http.MethodPost {
				posted++
			}
			w.WriteHeader(http.StatusNoContent)
		})
		file := filepath.Join(t.TempDir(), "taskmine-rejected")
		require.NoError(t, os.WriteFile(file, []byte("my-skill\nbug-login.md\n"), 0o600))

		err := cmdProposal(t.Context(), []string{"import-rejections", "--file", file, "--actor", "migrator"})
		require.NoError(t, err)
		assert.Equal(t, 2, posted)
	})

	t.Run("missing file flag is a validation error", func(t *testing.T) {
		setPipelineEnv(t, func(http.ResponseWriter, *http.Request) {})

		err := cmdProposal(t.Context(), []string{"import-rejections", "--actor", "a"})
		require.ErrorIs(t, err, domain.ErrValidation)
	})

	t.Run("nonexistent file is a validation error", func(t *testing.T) {
		setPipelineEnv(t, func(http.ResponseWriter, *http.Request) {})

		err := cmdProposal(t.Context(), []string{"import-rejections", "--file", filepath.Join(t.TempDir(), "nope"), "--actor", "a"})
		require.ErrorIs(t, err, domain.ErrValidation)
	})
}

func TestCmdProposalApply(t *testing.T) {
	t.Run("writes the flattened proposal into the work dir", func(t *testing.T) {
		workDir := t.TempDir()
		flattened := filepath.Join(t.TempDir(), "global__rules__r.md")
		require.NoError(t, os.WriteFile(flattened, []byte("rule\n"), 0o600))
		require.NoError(t, os.WriteFile(flattened+".path", []byte("global/rules/r.md"), 0o600))

		err := cmdProposalApply(t.Context(), []string{"--work-dir", workDir, "--file", flattened})
		require.NoError(t, err)

		content, err := os.ReadFile(filepath.Join(workDir, "global", "rules", "r.md"))
		require.NoError(t, err)
		assert.Equal(t, "rule\n", string(content))
	})
}
