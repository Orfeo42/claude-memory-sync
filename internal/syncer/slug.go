package syncer

import "strings"

const (
	homeCanonicalKey       = "HOME"
	homeCanonicalKeyPrefix = "HOME-"
)

func canonicalKey(slug, slugPrefix string) (string, bool) {
	if slug == slugPrefix {
		return homeCanonicalKey, true
	}

	prefixed := slugPrefix + "-"
	if strings.HasPrefix(slug, prefixed) {
		return homeCanonicalKeyPrefix + strings.TrimPrefix(slug, prefixed), true
	}

	return "", false
}

func reverseSlug(key, slugPrefix string) (string, bool) {
	if key == homeCanonicalKey {
		return slugPrefix, true
	}

	if strings.HasPrefix(key, homeCanonicalKeyPrefix) {
		return slugPrefix + "-" + strings.TrimPrefix(key, homeCanonicalKeyPrefix), true
	}

	return "", false
}
