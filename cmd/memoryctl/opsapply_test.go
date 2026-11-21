package main

import (
	"testing"

	"github.com/stretchr/testify/require"

	"claude-memory-sync/internal/domain"
)

func TestCmdOpsApply(t *testing.T) {
	t.Run("source flags combined with proposals-dir are rejected", func(t *testing.T) {
		err := cmdOpsApply(t.Context(), []string{"--proposals-dir", "x", "--source-flow", "f", "--source-run", "r"})
		require.ErrorIs(t, err, domain.ErrValidation)
	})

	t.Run("source-flow without source-run is rejected", func(t *testing.T) {
		err := cmdOpsApply(t.Context(), []string{"--source-flow", "f"})
		require.ErrorIs(t, err, domain.ErrValidation)
	})
}
