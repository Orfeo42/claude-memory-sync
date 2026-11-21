package hiddentext_test

import (
	"testing"

	"claude-memory-sync/internal/hiddentext"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFind(t *testing.T) {
	t.Run("plain ASCII text is clean", func(t *testing.T) {
		_, found := hiddentext.Find([]byte("hello\tworld\r\nline two\n"))
		assert.False(t, found)
	})

	t.Run("empty input is clean", func(t *testing.T) {
		_, found := hiddentext.Find(nil)
		assert.False(t, found)
	})

	t.Run("NUL control is rejected with offset", func(t *testing.T) {
		got, found := hiddentext.Find([]byte("ab\x00c"))
		require.True(t, found)
		assert.Equal(t, 2, got.Offset)
		assert.Equal(t, rune(0), got.Rune)
	})

	t.Run("backspace is rejected", func(t *testing.T) {
		_, found := hiddentext.Find([]byte("a\x08"))
		assert.True(t, found)
	})

	t.Run("vertical tab and form feed are rejected", func(t *testing.T) {
		_, foundVT := hiddentext.Find([]byte("a\x0B"))
		_, foundFF := hiddentext.Find([]byte("a\x0C"))
		assert.True(t, foundVT)
		assert.True(t, foundFF)
	})

	t.Run("C0 range boundaries 0x0E and 0x1F are rejected", func(t *testing.T) {
		_, foundLow := hiddentext.Find([]byte("a\x0E"))
		_, foundHigh := hiddentext.Find([]byte("a\x1F"))
		assert.True(t, foundLow)
		assert.True(t, foundHigh)
	})

	t.Run("escape is rejected", func(t *testing.T) {
		_, found := hiddentext.Find([]byte("a\x1b[31m"))
		assert.True(t, found)
	})

	t.Run("DEL is rejected", func(t *testing.T) {
		_, found := hiddentext.Find([]byte("a\x7f"))
		assert.True(t, found)
	})

	t.Run("tab LF CR are allowed", func(t *testing.T) {
		_, found := hiddentext.Find([]byte("\t\n\r"))
		assert.False(t, found)
	})

	t.Run("zero-width space is rejected", func(t *testing.T) {
		got, found := hiddentext.Find([]byte("a\u200Bb"))
		require.True(t, found)
		assert.Equal(t, "U+200B", got.CodePoint())
	})

	t.Run("word joiner range boundaries are rejected", func(t *testing.T) {
		_, foundLow := hiddentext.Find([]byte("a\u2060"))
		_, foundHigh := hiddentext.Find([]byte("a\u2064"))
		assert.True(t, foundLow)
		assert.True(t, foundHigh)
	})

	t.Run("U+2065 is allowed", func(t *testing.T) {
		_, found := hiddentext.Find([]byte("a\u2065"))
		assert.False(t, found)
	})

	t.Run("mongolian vowel separator is rejected", func(t *testing.T) {
		_, found := hiddentext.Find([]byte("a\u180E"))
		assert.True(t, found)
	})

	t.Run("combining grapheme joiner is rejected", func(t *testing.T) {
		_, found := hiddentext.Find([]byte("a\u034F"))
		assert.True(t, found)
	})

	t.Run("bidi embeddings and overrides are rejected", func(t *testing.T) {
		_, foundLow := hiddentext.Find([]byte("a\u202A"))
		_, foundHigh := hiddentext.Find([]byte("a\u202E"))
		assert.True(t, foundLow)
		assert.True(t, foundHigh)
	})

	t.Run("bidi isolates are rejected", func(t *testing.T) {
		_, foundLow := hiddentext.Find([]byte("a\u2066"))
		_, foundHigh := hiddentext.Find([]byte("a\u2069"))
		assert.True(t, foundLow)
		assert.True(t, foundHigh)
	})

	t.Run("tag characters are rejected at both boundaries", func(t *testing.T) {
		low, foundLow := hiddentext.Find([]byte("a\U000E0000"))
		_, foundHigh := hiddentext.Find([]byte("a\U000E007F"))
		require.True(t, foundLow)
		assert.Equal(t, "U+E0000", low.CodePoint())
		assert.True(t, foundHigh)
	})

	t.Run("rune just after tag range is allowed", func(t *testing.T) {
		_, found := hiddentext.Find([]byte("a\U000E0080"))
		assert.False(t, found)
	})

	t.Run("Hangul fillers are rejected", func(t *testing.T) {
		_, found115F := hiddentext.Find([]byte("a\u115F"))
		_, found1160 := hiddentext.Find([]byte("a\u1160"))
		_, found3164 := hiddentext.Find([]byte("a\u3164"))
		_, foundFFA0 := hiddentext.Find([]byte("a\uFFA0"))
		assert.True(t, found115F)
		assert.True(t, found1160)
		assert.True(t, found3164)
		assert.True(t, foundFFA0)
	})

	t.Run("BOM at offset 0 is allowed", func(t *testing.T) {
		_, found := hiddentext.Find([]byte{0xEF, 0xBB, 0xBF, 'h', 'i'})
		assert.False(t, found)
	})

	t.Run("BOM after offset 0 is rejected", func(t *testing.T) {
		got, found := hiddentext.Find([]byte{'a', 0xEF, 0xBB, 0xBF, 'b'})
		require.True(t, found)
		assert.Equal(t, 1, got.Offset)
		assert.Equal(t, "U+FEFF", got.CodePoint())
	})

	t.Run("LRM and RLM are allowed", func(t *testing.T) {
		_, found := hiddentext.Find([]byte("a\u200Eb\u200Fc"))
		assert.False(t, found)
	})

	t.Run("ZWJ between non-ASCII runes is allowed", func(t *testing.T) {
		_, found := hiddentext.Find([]byte("\u0915\u094D\u200D\u0937"))
		assert.False(t, found)
	})

	t.Run("emoji ZWJ sequence is allowed", func(t *testing.T) {
		_, found := hiddentext.Find([]byte("\U0001F468\u200D\U0001F469\u200D\U0001F467"))
		assert.False(t, found)
	})

	t.Run("ZWNJ between non-ASCII runes is allowed", func(t *testing.T) {
		_, found := hiddentext.Find([]byte("\u0645\u200C\u0646"))
		assert.False(t, found)
	})

	t.Run("ZWJ after ASCII is rejected", func(t *testing.T) {
		_, found := hiddentext.Find([]byte("a\u200D\U0001F469"))
		assert.True(t, found)
	})

	t.Run("ZWJ before ASCII is rejected", func(t *testing.T) {
		_, found := hiddentext.Find([]byte("\U0001F469\u200Da"))
		assert.True(t, found)
	})

	t.Run("ZWJ at start or end is rejected", func(t *testing.T) {
		_, foundStart := hiddentext.Find([]byte("\u200D\U0001F469"))
		_, foundEnd := hiddentext.Find([]byte("\U0001F469\u200D"))
		assert.True(t, foundStart)
		assert.True(t, foundEnd)
	})

	t.Run("ZWJ next to a rejected rune is rejected", func(t *testing.T) {
		got, found := hiddentext.Find([]byte("\U0001F469\u200D\u200B"))
		require.True(t, found)
		assert.Equal(t, "U+200D", got.CodePoint())
	})

	t.Run("ZWJ next to another ZWJ is rejected", func(t *testing.T) {
		_, found := hiddentext.Find([]byte("\U0001F469\u200D\u200D\U0001F467"))
		assert.True(t, found)
	})

	t.Run("variation selector 16 after emoji is allowed", func(t *testing.T) {
		_, found := hiddentext.Find([]byte("\u2764\uFE0F"))
		assert.False(t, found)
	})

	t.Run("variation selector 15 after non-ASCII is allowed", func(t *testing.T) {
		_, found := hiddentext.Find([]byte("\u2764\uFE0E"))
		assert.False(t, found)
	})

	t.Run("keycap sequence is allowed", func(t *testing.T) {
		_, found := hiddentext.Find([]byte("1\uFE0F\u20E3 #\uFE0F\u20E3 *\uFE0F\u20E3"))
		assert.False(t, found)
	})

	t.Run("variation selector after ASCII letter is rejected", func(t *testing.T) {
		got, found := hiddentext.Find([]byte("a\uFE0F"))
		require.True(t, found)
		assert.Equal(t, "U+FE0F", got.CodePoint())
	})

	t.Run("variation selector at start is rejected", func(t *testing.T) {
		_, found := hiddentext.Find([]byte("\uFE0F"))
		assert.True(t, found)
	})

	t.Run("selector after another selector is rejected", func(t *testing.T) {
		got, found := hiddentext.Find([]byte("\u2764\uFE0F\uFE0F"))
		require.True(t, found)
		assert.Equal(t, 6, got.Offset)
	})

	t.Run("selector after a rejected rune is rejected", func(t *testing.T) {
		_, found := hiddentext.Find([]byte("\u200B\uFE0F"))
		assert.True(t, found)
	})

	t.Run("invalid UTF-8 is ignored", func(t *testing.T) {
		_, found := hiddentext.Find([]byte{0xff, 0xfe, 'a', 0x80, 0xc3})
		assert.False(t, found)
	})

	t.Run("hidden rune after invalid bytes is still found", func(t *testing.T) {
		got, found := hiddentext.Find([]byte{0xff, 'a', 0xE2, 0x80, 0xAE})
		require.True(t, found)
		assert.Equal(t, 2, got.Offset)
	})

	t.Run("ZWJ after invalid byte is rejected", func(t *testing.T) {
		_, found := hiddentext.Find([]byte{0xff, 0xE2, 0x80, 0x8D, 0xF0, 0x9F, 0x91, 0xA9})
		assert.True(t, found)
	})

	t.Run("literal replacement character is allowed", func(t *testing.T) {
		_, found := hiddentext.Find([]byte("a\uFFFDb"))
		assert.False(t, found)
	})
}

func TestFindingCodePoint(t *testing.T) {
	t.Run("pads to four digits", func(t *testing.T) {
		assert.Equal(t, "U+0000", hiddentext.Finding{Rune: 0}.CodePoint())
	})

	t.Run("uppercase four digits", func(t *testing.T) {
		assert.Equal(t, "U+202E", hiddentext.Finding{Rune: 0x202E}.CodePoint())
	})

	t.Run("five digits for supplementary plane", func(t *testing.T) {
		assert.Equal(t, "U+E0041", hiddentext.Finding{Rune: 0xE0041}.CodePoint())
	})
}
