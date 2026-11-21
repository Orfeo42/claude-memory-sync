package settings

import (
	"context"
	"testing"

	"claude-memory-sync/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func strPtr(v string) *string {
	return &v
}

func TestNewService(t *testing.T) {
	t.Run("holds the repository", func(t *testing.T) {
		repo := &MockRepository{}
		assert.Same(t, repo, NewService(repo).repo)
	})
}

func TestServiceList(t *testing.T) {
	t.Run("hides secret values only", func(t *testing.T) {
		repo := &MockRepository{ListFunc: func(_ context.Context) ([]Setting, error) {
			return []Setting{
				{Key: "param.a", Value: strPtr("1"), Set: true},
				{Key: "secret.b", Value: strPtr("hush"), Set: true},
			}, nil
		}}

		got, err := NewService(repo).List(context.Background())
		require.NoError(t, err)
		require.Len(t, got, 2)
		assert.Equal(t, "1", *got[0].Value)
		assert.Nil(t, got[1].Value)
	})

	t.Run("propagates repository error", func(t *testing.T) {
		repo := &MockRepository{ListFunc: func(_ context.Context) ([]Setting, error) {
			return nil, domain.ErrInternal
		}}
		_, err := NewService(repo).List(context.Background())
		require.ErrorIs(t, err, domain.ErrInternal)
	})
}

func TestServiceGet(t *testing.T) {
	t.Run("returns secret value in full", func(t *testing.T) {
		repo := &MockRepository{GetFunc: func(_ context.Context, key Key) (Setting, error) {
			return Setting{Key: string(key), Value: strPtr("hush"), Set: true}, nil
		}}
		got, err := NewService(repo).Get(context.Background(), "secret.b")
		require.NoError(t, err)
		assert.Equal(t, "hush", *got.Value)
	})

	t.Run("bad key is a validation error and skips the repository", func(t *testing.T) {
		repo := &MockRepository{}
		_, err := NewService(repo).Get(context.Background(), "nope")
		require.ErrorIs(t, err, domain.ErrValidation)
		assert.Empty(t, repo.GetCalls())
	})

	t.Run("propagates not found", func(t *testing.T) {
		repo := &MockRepository{GetFunc: func(_ context.Context, _ Key) (Setting, error) {
			return Setting{}, domain.ErrNotFound
		}}
		_, err := NewService(repo).Get(context.Background(), "param.a")
		require.ErrorIs(t, err, domain.ErrNotFound)
	})
}

func TestServicePut(t *testing.T) {
	t.Run("passes parsed key value and actor", func(t *testing.T) {
		repo := &MockRepository{PutFunc: func(_ context.Context, req PutRequest) (Setting, error) {
			return Setting{Key: string(req.Key), Value: &req.Value, Set: true, UpdatedBy: req.Actor}, nil
		}}

		got, err := NewService(repo).Put(context.Background(), "param.a", "v", "me")
		require.NoError(t, err)
		assert.Equal(t, "v", *got.Value)
		require.Len(t, repo.PutCalls(), 1)
		assert.Equal(t, PutRequest{Key: "param.a", Value: "v", Actor: "me"}, repo.PutCalls()[0].Req)
	})

	t.Run("hides secret value in the response", func(t *testing.T) {
		repo := &MockRepository{PutFunc: func(_ context.Context, req PutRequest) (Setting, error) {
			return Setting{Key: string(req.Key), Value: &req.Value, Set: true}, nil
		}}
		got, err := NewService(repo).Put(context.Background(), "secret.a", "hush", "me")
		require.NoError(t, err)
		assert.Nil(t, got.Value)
	})

	t.Run("bad key is a validation error", func(t *testing.T) {
		repo := &MockRepository{}
		_, err := NewService(repo).Put(context.Background(), "BAD", "v", "me")
		require.ErrorIs(t, err, domain.ErrValidation)
		assert.Empty(t, repo.PutCalls())
	})

	t.Run("propagates repository error", func(t *testing.T) {
		repo := &MockRepository{PutFunc: func(_ context.Context, _ PutRequest) (Setting, error) {
			return Setting{}, domain.ErrInternal
		}}
		_, err := NewService(repo).Put(context.Background(), "param.a", "v", "me")
		require.ErrorIs(t, err, domain.ErrInternal)
	})
}

func TestServiceDelete(t *testing.T) {
	t.Run("passes parsed key and actor", func(t *testing.T) {
		repo := &MockRepository{DeleteFunc: func(_ context.Context, _ DeleteRequest) error { return nil }}
		require.NoError(t, NewService(repo).Delete(context.Background(), "param.a", "me"))
		require.Len(t, repo.DeleteCalls(), 1)
		assert.Equal(t, DeleteRequest{Key: "param.a", Actor: "me"}, repo.DeleteCalls()[0].Req)
	})

	t.Run("bad key is a validation error", func(t *testing.T) {
		repo := &MockRepository{}
		err := NewService(repo).Delete(context.Background(), "BAD", "me")
		require.ErrorIs(t, err, domain.ErrValidation)
		assert.Empty(t, repo.DeleteCalls())
	})
}

func TestServiceHistory(t *testing.T) {
	t.Run("passes key and limit", func(t *testing.T) {
		repo := &MockRepository{HistoryFunc: func(_ context.Context, _ Key, _ int) ([]Version, error) {
			return []Version{{CreatedBy: "me"}}, nil
		}}
		got, err := NewService(repo).History(context.Background(), "param.a", 7)
		require.NoError(t, err)
		assert.Len(t, got, 1)
		assert.Equal(t, 7, repo.HistoryCalls()[0].Limit)
	})

	t.Run("bad key is a validation error", func(t *testing.T) {
		repo := &MockRepository{}
		_, err := NewService(repo).History(context.Background(), "BAD", 7)
		require.ErrorIs(t, err, domain.ErrValidation)
		assert.Empty(t, repo.HistoryCalls())
	})
}
