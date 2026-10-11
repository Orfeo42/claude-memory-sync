package gitutil

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWithFileLock(t *testing.T) {
	t.Run("serializes concurrent critical sections", func(t *testing.T) {
		lockPath := filepath.Join(t.TempDir(), "test.lock")

		const goroutines = 20
		counter := 0
		var wg sync.WaitGroup
		errs := make([]error, goroutines)

		for i := range goroutines {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				errs[idx] = WithFileLock(lockPath, func() error {
					current := counter
					time.Sleep(time.Millisecond)
					counter = current + 1
					return nil
				})
			}(i)
		}

		wg.Wait()

		for _, err := range errs {
			require.NoError(t, err)
		}
		assert.Equal(t, goroutines, counter)
	})

	t.Run("propagates the callback error", func(t *testing.T) {
		lockPath := filepath.Join(t.TempDir(), "test.lock")
		sentinelErr := errors.New("boom")

		err := WithFileLock(lockPath, func() error {
			return sentinelErr
		})

		require.Error(t, err)
		assert.ErrorIs(t, err, sentinelErr)
	})

	t.Run("creates missing parent dir", func(t *testing.T) {
		lockPath := filepath.Join(t.TempDir(), "state", "nested", "test.lock")

		called := false
		err := WithFileLock(lockPath, func() error {
			called = true
			return nil
		})

		require.NoError(t, err)
		assert.True(t, called)
		assert.FileExists(t, lockPath)
	})
}

func TestIsAncestor(t *testing.T) {
	ctx := context.Background()

	repo := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		out, err := exec.Command("git", append([]string{"-C", repo}, args...)...).CombinedOutput()
		require.NoErrorf(t, err, "git %v: %s", args, out)
	}
	git("init", "--initial-branch=main")
	git("config", "user.name", "t")
	git("config", "user.email", "t@t")
	git("commit", "--allow-empty", "-m", "first")
	git("commit", "--allow-empty", "-m", "second")

	t.Run("parent is ancestor of child", func(t *testing.T) {
		ok, err := IsAncestor(ctx, repo, "HEAD~1", "HEAD")
		require.NoError(t, err)
		assert.True(t, ok)
	})

	t.Run("child is not ancestor of parent", func(t *testing.T) {
		ok, err := IsAncestor(ctx, repo, "HEAD", "HEAD~1")
		require.NoError(t, err)
		assert.False(t, ok)
	})

	t.Run("bad ref returns error", func(t *testing.T) {
		_, err := IsAncestor(ctx, repo, "nope", "HEAD")
		require.Error(t, err)
	})
}
