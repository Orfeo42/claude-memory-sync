package gitutil

import (
	"errors"
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
}
