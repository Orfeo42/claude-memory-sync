package hiddentext

import (
	"fmt"
	"unicode/utf8"
)

const (
	zwnj         rune = 0x200C
	zwj          rune = 0x200D
	vs15         rune = 0xFE0E
	vs16         rune = 0xFE0F
	bom          rune = 0xFEFF
	maxASCII     rune = 0x7F
	noRune       rune = -1
	minCodeWidth      = 4
)

type Finding struct {
	Offset int
	Rune   rune
}

func (f Finding) CodePoint() string {
	return fmt.Sprintf("U+%0*X", minCodeWidth, f.Rune)
}

func Find(data []byte) (Finding, bool) {
	prev := noRune
	for offset := 0; offset < len(data); {
		r, size := utf8.DecodeRune(data[offset:])
		if r == utf8.RuneError && size == 1 {
			prev = noRune
			offset++
			continue
		}

		next := noRune
		nr, nsize := utf8.DecodeRune(data[offset+size:])
		if nr != utf8.RuneError || nsize > 1 {
			next = nr
		}

		if isOffending(r, prev, next, offset) {
			return Finding{Offset: offset, Rune: r}, true
		}

		prev = r
		offset += size
	}

	return Finding{}, false
}

func isOffending(r, prev, next rune, offset int) bool {
	switch r {
	case bom:
		return offset != 0
	case zwnj, zwj:
		return !isPlainNonASCII(prev) || !isPlainNonASCII(next)
	case vs15, vs16:
		return !isPlainNonASCII(prev) && !isKeycapBase(prev)
	}

	return isRejected(r)
}

func isRejected(r rune) bool {
	switch {
	case r <= 0x08, r == 0x0B, r == 0x0C, r >= 0x0E && r <= 0x1F, r == 0x7F:
		return true
	case r >= 0xE0000 && r <= 0xE007F:
		return true
	}

	return isInvisibleFormat(r)
}

func isInvisibleFormat(r rune) bool {
	switch {
	case r == 0x200B, r >= 0x2060 && r <= 0x2064, r == 0x180E, r == 0x034F:
		return true
	case r >= 0x202A && r <= 0x202E, r >= 0x2066 && r <= 0x2069:
		return true
	}

	return r == 0x115F || r == 0x1160 || r == 0x3164 || r == 0xFFA0
}

func isPlainNonASCII(r rune) bool {
	if r <= maxASCII {
		return false
	}

	return !isRejected(r) && r != zwnj && r != zwj && r != vs15 && r != vs16 && r != bom
}

func isKeycapBase(r rune) bool {
	return r >= '0' && r <= '9' || r == '#' || r == '*'
}
