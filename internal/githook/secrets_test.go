package githook_test

import (
	"strings"
	"testing"

	"claude-memory-sync/internal/githook"

	"github.com/stretchr/testify/assert"
)

func TestContainsSecret(t *testing.T) {
	t.Run("matches a secret-shaped token", func(t *testing.T) {
		fakeToken := "gh" + "p_" + strings.Repeat("a", 24)
		content := []byte("notes\ntoken here: " + fakeToken + "\n")

		assert.True(t, githook.ContainsSecret(content))
	})

	t.Run("does not match clean content", func(t *testing.T) {
		content := []byte("just some regular prose about tokens of appreciation\n")

		assert.False(t, githook.ContainsSecret(content))
	})
}
