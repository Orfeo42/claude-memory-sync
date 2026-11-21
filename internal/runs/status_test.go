package runs

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"claude-memory-sync/internal/domain"
)

func TestParseTrigger(t *testing.T) {
	t.Run("accepts schedule", func(t *testing.T) {
		got, err := ParseTrigger("schedule")
		require.NoError(t, err)
		assert.Equal(t, TriggerSchedule, got)
	})

	t.Run("accepts manual", func(t *testing.T) {
		got, err := ParseTrigger("manual")
		require.NoError(t, err)
		assert.Equal(t, TriggerManual, got)
	})

	t.Run("accepts event", func(t *testing.T) {
		got, err := ParseTrigger("event")
		require.NoError(t, err)
		assert.Equal(t, TriggerEvent, got)
	})

	t.Run("rejects unknown", func(t *testing.T) {
		_, err := ParseTrigger("cron")
		require.ErrorIs(t, err, domain.ErrValidation)
	})
}

func TestParseStatus(t *testing.T) {
	t.Run("accepts running", func(t *testing.T) {
		got, err := ParseStatus("running")
		require.NoError(t, err)
		assert.Equal(t, StatusRunning, got)
	})

	t.Run("accepts succeeded", func(t *testing.T) {
		got, err := ParseStatus("succeeded")
		require.NoError(t, err)
		assert.Equal(t, StatusSucceeded, got)
	})

	t.Run("accepts failed", func(t *testing.T) {
		got, err := ParseStatus("failed")
		require.NoError(t, err)
		assert.Equal(t, StatusFailed, got)
	})

	t.Run("accepts partial", func(t *testing.T) {
		got, err := ParseStatus("partial")
		require.NoError(t, err)
		assert.Equal(t, StatusPartial, got)
	})

	t.Run("accepts interrupted", func(t *testing.T) {
		got, err := ParseStatus("interrupted")
		require.NoError(t, err)
		assert.Equal(t, StatusInterrupted, got)
	})

	t.Run("rejects unknown", func(t *testing.T) {
		_, err := ParseStatus("paused")
		require.ErrorIs(t, err, domain.ErrValidation)
	})

	t.Run("rejects empty", func(t *testing.T) {
		_, err := ParseStatus("")
		require.ErrorIs(t, err, domain.ErrValidation)
	})
}

func TestParseStepStatus(t *testing.T) {
	t.Run("accepts succeeded", func(t *testing.T) {
		got, err := ParseStepStatus("succeeded")
		require.NoError(t, err)
		assert.Equal(t, StepSucceeded, got)
	})

	t.Run("accepts failed", func(t *testing.T) {
		got, err := ParseStepStatus("failed")
		require.NoError(t, err)
		assert.Equal(t, StepFailed, got)
	})

	t.Run("accepts skipped", func(t *testing.T) {
		got, err := ParseStepStatus("skipped")
		require.NoError(t, err)
		assert.Equal(t, StepSkipped, got)
	})

	t.Run("accepts lossy", func(t *testing.T) {
		got, err := ParseStepStatus("lossy")
		require.NoError(t, err)
		assert.Equal(t, StepLossy, got)
	})

	t.Run("rejects run-only status", func(t *testing.T) {
		_, err := ParseStepStatus("running")
		require.ErrorIs(t, err, domain.ErrValidation)
	})
}
