package gitserver

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"claude-memory-sync/internal/domain"
)

const (
	gitPathPrefix     = "/git/"
	infoRefsSuffix    = "/info/refs"
	uploadPackSuffix  = "/git-upload-pack"
	receivePackSuffix = "/git-receive-pack"
)

type handler struct {
	reposRoot   string
	hookCommand string
}

func New(reposRoot string) http.Handler {
	exe, err := os.Executable()
	if err != nil {
		slog.Error("failed to resolve server executable path for git hook installation", slog.String("error", err.Error()))
		exe = ""
	}

	return newHandler(reposRoot, exe)
}

func newHandler(reposRoot, hookCommand string) *handler {
	return &handler{reposRoot: reposRoot, hookCommand: hookCommand}
}

type parsedRequest struct {
	repo       string
	service    string
	isInfoRefs bool
}

func parseRequestPath(path string) (parsedRequest, bool) {
	rest, ok := strings.CutPrefix(path, gitPathPrefix)
	if !ok {
		return parsedRequest{}, false
	}

	if repo, ok := strings.CutSuffix(rest, infoRefsSuffix); ok {
		return parsedRequest{repo: repo, isInfoRefs: true}, true
	}

	if repo, ok := strings.CutSuffix(rest, uploadPackSuffix); ok {
		return parsedRequest{repo: repo, service: serviceUploadPack}, true
	}

	if repo, ok := strings.CutSuffix(rest, receivePackSuffix); ok {
		return parsedRequest{repo: repo, service: serviceReceivePack}, true
	}

	return parsedRequest{}, false
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	parsed, ok := parseRequestPath(r.URL.Path)
	if !ok || !validRepoName(parsed.repo) {
		http.NotFound(w, r)
		return
	}

	if parsed.isInfoRefs {
		h.handleInfoRefs(ctx, w, r, parsed.repo)
		return
	}

	h.handlePack(ctx, w, r, parsed.repo, parsed.service)
}

func (h *handler) handleInfoRefs(ctx context.Context, w http.ResponseWriter, r *http.Request, repo string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	service, ok := serviceName(r.URL.Query().Get("service"))
	if !ok {
		h.writeError(ctx, w, domain.Error(errInvalidService, "invalid or missing service query parameter"))
		return
	}

	if err := checkPolicy(repo, service); err != nil {
		h.writeError(ctx, w, err)
		return
	}

	dir, err := h.resolveRepoDir(ctx, repo, service)
	if err != nil {
		h.writeError(ctx, w, err)
		return
	}

	if err := writeInfoRefs(ctx, w, dir, service); err != nil {
		domain.LogError(ctx, "failed to serve info/refs", err)
	}
}

func (h *handler) handlePack(ctx context.Context, w http.ResponseWriter, r *http.Request, repo, service string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := checkPolicy(repo, service); err != nil {
		h.writeError(ctx, w, err)
		return
	}

	dir, err := h.resolveRepoDir(ctx, repo, service)
	if err != nil {
		h.writeError(ctx, w, err)
		return
	}

	if err := servePack(ctx, w, r, dir, service); err != nil {
		domain.LogError(ctx, "failed to serve git pack request", err)
	}
}

func statusForError(err error) int {
	switch {
	case errors.Is(err, errRepoNotFound):
		return http.StatusNotFound
	case errors.Is(err, errInvalidService):
		return http.StatusBadRequest
	case errors.Is(err, errCanonicalWriteForbidden):
		return http.StatusForbidden
	default:
		return http.StatusInternalServerError
	}
}

func (h *handler) writeError(ctx context.Context, w http.ResponseWriter, err error) {
	status := statusForError(err)

	if status >= http.StatusInternalServerError {
		domain.LogError(ctx, "git request failed", err)
	} else {
		domain.LogWarn(ctx, "git request rejected", err)
	}

	http.Error(w, domain.UserMessage(err), status)
}
