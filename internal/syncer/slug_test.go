package syncer

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCanonicalKey(t *testing.T) {
	t.Run("exact prefix match maps to HOME", func(t *testing.T) {
		key, ok := canonicalKey("-home-orfeo42", "-home-orfeo42")
		require.True(t, ok)
		assert.Equal(t, "HOME", key)
	})

	t.Run("prefixed slug maps to HOME-suffix", func(t *testing.T) {
		key, ok := canonicalKey("-home-orfeo42-sviluppo-project-x", "-home-orfeo42")
		require.True(t, ok)
		assert.Equal(t, "HOME-sviluppo-project-x", key)
	})

	t.Run("foreign prefix is skipped", func(t *testing.T) {
		_, ok := canonicalKey("C--Users-x-project", "-home-orfeo42")
		assert.False(t, ok)
	})

	t.Run("prefix substring without separator is skipped", func(t *testing.T) {
		_, ok := canonicalKey("-home-orfeo42x", "-home-orfeo42")
		assert.False(t, ok)
	})
}

func TestReverseSlug(t *testing.T) {
	t.Run("HOME maps back to slug prefix", func(t *testing.T) {
		slug, ok := reverseSlug("HOME", "-home-orfeo42")
		require.True(t, ok)
		assert.Equal(t, "-home-orfeo42", slug)
	})

	t.Run("HOME-suffix maps back to prefixed slug", func(t *testing.T) {
		slug, ok := reverseSlug("HOME-sviluppo-project-x", "-home-orfeo42")
		require.True(t, ok)
		assert.Equal(t, "-home-orfeo42-sviluppo-project-x", slug)
	})

	t.Run("unknown key is rejected", func(t *testing.T) {
		_, ok := reverseSlug("OTHER", "-home-orfeo42")
		assert.False(t, ok)
	})

	t.Run("canonicalKey and reverseSlug round-trip", func(t *testing.T) {
		slugPrefix := "-home-orfeo42"
		originalSlug := "-home-orfeo42-sviluppo-project-x"

		key, ok := canonicalKey(originalSlug, slugPrefix)
		require.True(t, ok)

		gotSlug, ok := reverseSlug(key, slugPrefix)
		require.True(t, ok)
		assert.Equal(t, originalSlug, gotSlug)
	})
}
