package postgres

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func tablesPresent(t *testing.T) bool {
	t.Helper()
	var present bool
	err := testDB.GetContext(context.Background(), &present,
		"SELECT to_regclass('proposal') IS NOT NULL AND to_regclass('proposal_file') IS NOT NULL AND to_regclass('proposal_event') IS NOT NULL AND to_regclass('rejection') IS NOT NULL")
	require.NoError(t, err)
	return present
}

func TestMigrate(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test needs postgres")
	}

	t.Run("up is idempotent", func(t *testing.T) {
		require.NoError(t, Migrate(context.Background(), testDB))
		require.NoError(t, Migrate(context.Background(), testDB))
		assert.True(t, tablesPresent(t))
	})

	t.Run("down then up restores schema", func(t *testing.T) {
		provider, err := newProvider(testDB)
		require.NoError(t, err)

		_, err = provider.DownTo(context.Background(), 0)
		require.NoError(t, err)
		assert.False(t, tablesPresent(t))

		require.NoError(t, Migrate(context.Background(), testDB))
		assert.True(t, tablesPresent(t))
	})
}
