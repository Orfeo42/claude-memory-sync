package clientid

import "regexp"

var pattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

func Valid(id string) bool {
	return id != "" && pattern.MatchString(id)
}
