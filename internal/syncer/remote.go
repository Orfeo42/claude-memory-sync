package syncer

import "strings"

func clientRemoteURL(cfg Config) string {
	return strings.TrimSuffix(cfg.ServerURL, "/") + "/git/clients/" + cfg.ClientID + ".git"
}

func canonicalRemoteURL(cfg Config) string {
	return strings.TrimSuffix(cfg.ServerURL, "/") + "/git/canonical.git"
}
