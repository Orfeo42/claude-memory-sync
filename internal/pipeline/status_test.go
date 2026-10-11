package pipeline

import (
	"testing"

	"claude-memory-sync/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseStatus(t *testing.T) {
	t.Run("accepts every known status", func(t *testing.T) {
		got, err := ParseStatus("pending")
		require.NoError(t, err)
		assert.Equal(t, StatusPending, got)

		got, err = ParseStatus("approved")
		require.NoError(t, err)
		assert.Equal(t, StatusApproved, got)

		got, err = ParseStatus("applying")
		require.NoError(t, err)
		assert.Equal(t, StatusApplying, got)

		got, err = ParseStatus("applied")
		require.NoError(t, err)
		assert.Equal(t, StatusApplied, got)

		got, err = ParseStatus("rejected")
		require.NoError(t, err)
		assert.Equal(t, StatusRejected, got)

		got, err = ParseStatus("expired")
		require.NoError(t, err)
		assert.Equal(t, StatusExpired, got)

		got, err = ParseStatus("failed")
		require.NoError(t, err)
		assert.Equal(t, StatusFailed, got)
	})

	t.Run("rejects unknown status", func(t *testing.T) {
		_, err := ParseStatus("bogus")
		require.ErrorIs(t, err, domain.ErrValidation)
	})

	t.Run("rejects empty status", func(t *testing.T) {
		_, err := ParseStatus("")
		require.ErrorIs(t, err, domain.ErrValidation)
	})
}

func TestCanTransition(t *testing.T) {
	t.Run("pending to approved", func(t *testing.T) {
		assert.True(t, CanTransition(StatusPending, StatusApproved))
	})

	t.Run("pending to rejected", func(t *testing.T) {
		assert.True(t, CanTransition(StatusPending, StatusRejected))
	})

	t.Run("pending to expired", func(t *testing.T) {
		assert.True(t, CanTransition(StatusPending, StatusExpired))
	})

	t.Run("approved to applying", func(t *testing.T) {
		assert.True(t, CanTransition(StatusApproved, StatusApplying))
	})

	t.Run("applying to applied failed and back to approved", func(t *testing.T) {
		assert.True(t, CanTransition(StatusApplying, StatusApplied))
		assert.True(t, CanTransition(StatusApplying, StatusFailed))
		assert.True(t, CanTransition(StatusApplying, StatusApproved))
	})

	t.Run("failed to approved", func(t *testing.T) {
		assert.True(t, CanTransition(StatusFailed, StatusApproved))
	})

	t.Run("terminal statuses have no exits", func(t *testing.T) {
		assert.False(t, CanTransition(StatusApplied, StatusApproved))
		assert.False(t, CanTransition(StatusRejected, StatusApproved))
		assert.False(t, CanTransition(StatusExpired, StatusApproved))
		assert.False(t, CanTransition(StatusApplied, StatusPending))
	})

	t.Run("pending cannot jump to applied", func(t *testing.T) {
		assert.False(t, CanTransition(StatusPending, StatusApplied))
	})

	t.Run("unknown source status", func(t *testing.T) {
		assert.False(t, CanTransition(Status("bogus"), StatusApproved))
	})
}

func TestOpenStatuses(t *testing.T) {
	t.Run("returns pending approved applying", func(t *testing.T) {
		assert.Equal(t, []Status{StatusPending, StatusApproved, StatusApplying}, OpenStatuses())
	})
}
