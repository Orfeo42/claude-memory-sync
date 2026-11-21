package proposalapply

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExpire(t *testing.T) {
	t.Run("converts the duration to hours and reports the count", func(t *testing.T) {
		mock := &MockPipeline{
			ExpireFunc: func(_ context.Context, _ string, _ int) (int, error) { return 3, nil },
		}

		result, err := Expire(t.Context(), mock, "reaper", 72*time.Hour)
		require.NoError(t, err)
		assert.Equal(t, 3, result.Expired)

		calls := mock.ExpireCalls()
		require.Len(t, calls, 1)
		assert.Equal(t, "reaper", calls[0].Actor)
		assert.Equal(t, 72, calls[0].OlderThanHours)
	})
}
