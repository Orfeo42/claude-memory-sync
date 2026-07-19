package api

import "regexp"

var clientIDPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

func validClientID(id string) bool {
	return id != "" && clientIDPattern.MatchString(id)
}
