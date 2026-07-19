package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRunOnceEnabled(t *testing.T) {
	t.Run("true when MEMORY_RUN_ONCE is true", func(t *testing.T) {
		t.Setenv("MEMORY_RUN_ONCE", "true")
		assert.True(t, runOnceEnabled())
	})

	t.Run("false when MEMORY_RUN_ONCE is unset", func(t *testing.T) {
		t.Setenv("MEMORY_RUN_ONCE", "")
		assert.False(t, runOnceEnabled())
	})

	t.Run("false when MEMORY_RUN_ONCE has an unexpected value", func(t *testing.T) {
		t.Setenv("MEMORY_RUN_ONCE", "1")
		assert.False(t, runOnceEnabled())
	})
}
