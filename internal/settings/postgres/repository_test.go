package postgres

import (
	"context"
	"testing"

	"claude-memory-sync/internal/domain"
	"claude-memory-sync/internal/settings"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mustPut(ctx context.Context, t *testing.T, repo *Repository, key, value string) settings.Setting {
	t.Helper()
	saved, err := repo.Put(ctx, settings.PutRequest{Key: settings.Key(key), Value: value, Actor: "tester"})
	require.NoError(t, err)
	return saved
}

func TestRepositoryPut(t *testing.T) {
	ctx := context.Background()

	t.Run("inserts then updates the same key", func(t *testing.T) {
		repo := setup(ctx, t)

		first := mustPut(ctx, t, repo, "param.a", "one")
		second := mustPut(ctx, t, repo, "param.a", "two")

		assert.Equal(t, "one", *first.Value)
		assert.Equal(t, "two", *second.Value)
		assert.True(t, second.Set)
		assert.Equal(t, "tester", second.UpdatedBy)
		all, err := repo.List(ctx)
		require.NoError(t, err)
		assert.Len(t, all, 1)
	})

	t.Run("writes one version row per put with the value", func(t *testing.T) {
		repo := setup(ctx, t)

		mustPut(ctx, t, repo, "param.a", "one")
		mustPut(ctx, t, repo, "param.a", "two")

		versions, err := repo.History(ctx, "param.a", 10)
		require.NoError(t, err)
		require.Len(t, versions, 2)
		assert.Equal(t, "two", *versions[0].Value)
		assert.Equal(t, "one", *versions[1].Value)
		assert.False(t, versions[0].Deleted)
	})

	t.Run("secret keeps the value but audits NULL", func(t *testing.T) {
		repo := setup(ctx, t)

		mustPut(ctx, t, repo, "secret.tok", "hush")

		got, err := repo.Get(ctx, "secret.tok")
		require.NoError(t, err)
		assert.Equal(t, "hush", *got.Value)
		versions, err := repo.History(ctx, "secret.tok", 10)
		require.NoError(t, err)
		require.Len(t, versions, 1)
		assert.Nil(t, versions[0].Value)
	})

	t.Run("rolls back the setting when the version write fails", func(t *testing.T) {
		repo := setup(ctx, t)
		_, err := testDB.ExecContext(ctx, "ALTER TABLE setting_versions RENAME TO setting_versions_x")
		require.NoError(t, err)
		t.Cleanup(func() {
			_, cleanupErr := testDB.ExecContext(ctx, "ALTER TABLE setting_versions_x RENAME TO setting_versions")
			require.NoError(t, cleanupErr)
		})

		_, err = repo.Put(ctx, settings.PutRequest{Key: "param.a", Value: "v", Actor: "tester"})
		require.Error(t, err)

		_, err = repo.Get(ctx, "param.a")
		require.ErrorIs(t, err, domain.ErrNotFound)
	})
}

func TestRepositoryGet(t *testing.T) {
	ctx := context.Background()

	t.Run("returns the stored row", func(t *testing.T) {
		repo := setup(ctx, t)
		mustPut(ctx, t, repo, "prompt.intake", "hello")

		got, err := repo.Get(ctx, "prompt.intake")
		require.NoError(t, err)
		assert.Equal(t, "prompt.intake", got.Key)
		assert.Equal(t, "hello", *got.Value)
	})

	t.Run("unset key is not found", func(t *testing.T) {
		repo := setup(ctx, t)
		_, err := repo.Get(ctx, "prompt.missing")
		require.ErrorIs(t, err, domain.ErrNotFound)
	})
}

func TestRepositoryList(t *testing.T) {
	ctx := context.Background()

	t.Run("orders by key", func(t *testing.T) {
		repo := setup(ctx, t)
		mustPut(ctx, t, repo, "schedule.b", "1")
		mustPut(ctx, t, repo, "param.a", "2")

		all, err := repo.List(ctx)
		require.NoError(t, err)
		require.Len(t, all, 2)
		assert.Equal(t, "param.a", all[0].Key)
		assert.Equal(t, "schedule.b", all[1].Key)
	})

	t.Run("empty table gives empty slice", func(t *testing.T) {
		repo := setup(ctx, t)
		all, err := repo.List(ctx)
		require.NoError(t, err)
		assert.Empty(t, all)
		assert.NotNil(t, all)
	})
}

func TestRepositoryDelete(t *testing.T) {
	ctx := context.Background()

	t.Run("removes the row and writes a deleted version", func(t *testing.T) {
		repo := setup(ctx, t)
		mustPut(ctx, t, repo, "param.a", "one")

		require.NoError(t, repo.Delete(ctx, settings.DeleteRequest{Key: "param.a", Actor: "remover"}))

		_, err := repo.Get(ctx, "param.a")
		require.ErrorIs(t, err, domain.ErrNotFound)
		versions, err := repo.History(ctx, "param.a", 10)
		require.NoError(t, err)
		require.Len(t, versions, 2)
		assert.True(t, versions[0].Deleted)
		assert.Nil(t, versions[0].Value)
		assert.Equal(t, "remover", versions[0].CreatedBy)
	})

	t.Run("unset key is a no-op without a version row", func(t *testing.T) {
		repo := setup(ctx, t)

		require.NoError(t, repo.Delete(ctx, settings.DeleteRequest{Key: "param.a", Actor: "remover"}))

		versions, err := repo.History(ctx, "param.a", 10)
		require.NoError(t, err)
		assert.Empty(t, versions)
	})

	t.Run("secret delete audits NULL", func(t *testing.T) {
		repo := setup(ctx, t)
		mustPut(ctx, t, repo, "secret.tok", "hush")

		require.NoError(t, repo.Delete(ctx, settings.DeleteRequest{Key: "secret.tok", Actor: "remover"}))

		versions, err := repo.History(ctx, "secret.tok", 10)
		require.NoError(t, err)
		require.Len(t, versions, 2)
		assert.Nil(t, versions[0].Value)
		assert.Nil(t, versions[1].Value)
	})
}

func TestRepositoryHistory(t *testing.T) {
	ctx := context.Background()

	t.Run("limits and orders newest first", func(t *testing.T) {
		repo := setup(ctx, t)
		mustPut(ctx, t, repo, "param.a", "1")
		mustPut(ctx, t, repo, "param.a", "2")
		mustPut(ctx, t, repo, "param.a", "3")

		versions, err := repo.History(ctx, "param.a", 2)
		require.NoError(t, err)
		require.Len(t, versions, 2)
		assert.Equal(t, "3", *versions[0].Value)
		assert.Equal(t, "2", *versions[1].Value)
	})

	t.Run("is scoped to the key", func(t *testing.T) {
		repo := setup(ctx, t)
		mustPut(ctx, t, repo, "param.a", "1")
		mustPut(ctx, t, repo, "param.b", "2")

		versions, err := repo.History(ctx, "param.b", 10)
		require.NoError(t, err)
		assert.Len(t, versions, 1)
	})
}
