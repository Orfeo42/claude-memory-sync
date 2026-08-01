package gitserver

import (
	"log/slog"

	"claude-memory-sync/internal/domain"
)

type sentinelError string

func (e sentinelError) Error() string {
	return string(e)
}

const (
	errRepoNotFound            sentinelError = "repository not found"
	errInvalidService          sentinelError = "invalid or missing service parameter"
	errCanonicalWriteForbidden sentinelError = "canonical repository is fetch-only over http"
	errHookCommandUnresolved   sentinelError = "server executable path could not be resolved for hook installation"

	serviceUploadPack  = "upload-pack"
	serviceReceivePack = "receive-pack"
)

func serviceName(service string) (string, bool) {
	switch service {
	case "git-upload-pack":
		return serviceUploadPack, true
	case "git-receive-pack":
		return serviceReceivePack, true
	default:
		return "", false
	}
}

func checkPolicy(repo, service string) error {
	if isCanonicalRepo(repo) && service == serviceReceivePack {
		return domain.Error(errCanonicalWriteForbidden, "canonical repository is fetch-only over http", slog.String("repo", repo))
	}

	return nil
}

func errAttrs(repo string) []slog.Attr {
	return []slog.Attr{slog.String("repo", repo)}
}

func errAttrsClient(clientID string) []slog.Attr {
	return []slog.Attr{slog.String("client_id", clientID)}
}
