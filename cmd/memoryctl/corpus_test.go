package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCmdCorpus(t *testing.T) {
	t.Run("missing subcommand is rejected", func(t *testing.T) {
		err := cmdCorpus(t.Context(), nil)
		require.ErrorIs(t, err, errUnknownCorpusSubcommand)
	})

	t.Run("unknown subcommand is rejected", func(t *testing.T) {
		err := cmdCorpus(t.Context(), []string{"nope"})
		require.ErrorIs(t, err, errUnknownCorpusSubcommand)
	})
}
