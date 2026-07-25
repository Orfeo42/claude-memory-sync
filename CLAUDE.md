# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```sh
go build ./...                      # build everything
go test -short ./...                # unit tests (all packages)
go test -short -run 'TestName' ./internal/store/   # single test
golangci-lint run ./...             # lint (strict, must be 0 issues)
golangci-lint run --fix ./internal/api/            # lint + autofix one package
golangci-lint fmt                   # format (gofumpt + goimports via config)
go generate ./...                   # regenerate moq mocks (store_mock.go, httpclient_mock.go)
./test/e2e.sh                       # end-to-end test — requires Docker, builds both images
```

golangci-lint is **v2** (config `version: "2"` in `.golangci.yml`). The VS Code and Claude hook autofix layers both run this same binary/config — do not introduce a second formatter or lint config.

## Architecture

Hub-and-spoke sync of Claude Code memory files (`~/.claude` content) across machines. This repo is **code only** — memory content never lives here. Two binaries, both containerized (`build/*.Dockerfile`, `deploy/*/compose.yaml`):

- **`cmd/server`** — hub API on the always-on home box. `internal/api` (huma v2 on net/http, bearer-token auth middleware) over `internal/store`: a git-backed filesystem store (`gitStore`) that shells out to `git` (`store/git.go`), one commit per write/delete, branch `storage`. Layout under the storage root: `canonical/` (merged truth) and `clients/<client-id>/` (per-machine snapshots).
- **`cmd/agent`** — per-machine Docker agent. `internal/syncer.Agent.RunCycle` runs on a ticker: `scanLocal` whitelists files under `~/.claude` (mounted at `/claude`), then **up-sync** (local vs saved client-base manifest → PUT/DELETE to server) and **down-sync** (canonical vs saved canonical-base → write/delete local files). Base manifests persist in the state dir between cycles.
- **`internal/manifest`** — content-hash manifests + `manifest.Diff`, the 3-way diff engine both directions of sync rely on. Change kinds (`LocalOnlyChange`, `LocalDelete`, …) drive what each sync step does.
- **`internal/syncer/slug.go` / `localpath.go`** — mapping between local `~/.claude` paths and namespaced sync paths (slug prefix per machine identifies per-project memory dirs).

### Error handling (`internal/domain`)

Ported pseudo-stack design; all new error paths must use it:

- Sentinels are **constants**: `type sentinelError string` with `const ErrNotFound/ErrUnauthorized/ErrValidation/ErrConflict/ErrUpstream/ErrInternal`.
- Wrap with `domain.Error(err, msg, attrs...)` / `domain.WrapError(err, attrs...)` — nil-safe, auto-appends `origin` (`file:line`) attr. Structured context (namespace, path, client_id) travels **on the error**, not in intermediate logs.
- Log **once at the boundary** only: server boundary in `internal/api/errors.go`, agent boundary in `cmd/agent` `runCycle`. `domain.LogError` / `domain.LogWarn` flatten `domain.ErrorAttrs` (origins as pseudo-stack) into the record. Never log-then-return mid-chain.
- Level by fault source: upstream/dependency fault (`errors.Is(err, domain.ErrUpstream)`, `UpstreamError`, `UpstreamUnreachableError`) → Warn; own fault → Error; expected 4xx → not logged.
- HTTP mapping via `domain.StatusCodeFromError` + `domain.UserMessage` — upstream 5xx surfaces as 502, never blanket 500.

## Lint/style constraints baked into `.golangci.yml`

- sloglint: snake_case keys, static messages, `*Context` logger variants required, typed attrs.
- depguard bans `log` and `pkg/errors`; slog only.
- gofumpt needs `module-path: claude-memory-sync` in config (already set) — removing it causes phantom "not properly formatted" findings.
- No doc comments required (revive `exported` intentionally off) — repo convention is no comments at all.
- Mocks generated with moq (`//go:generate` directives in `store.go`, `httpclient.go`); regenerate rather than hand-edit `*_mock.go`.

## Ground rules (from README — apply to all phases)

- Sync is **whitelist, not blacklist**: only explicitly listed paths leave `~/.claude`. Never sync `sessions/`, `ide/`, `daemon/`, `.credentials.json`, `cache/`, `paste-cache/`, `telemetry/`, `history.jsonl`, `shell-snapshots/`, `jobs/`, `tasks/`, `file-history/`.
- GitHub is backup only (server-side daily push), never the sync path.
- Machine list is open-ended — never hardcode a fixed set of hosts.
- Phase docs live in `.docs/` (00–06); phase 1 (hub sync) is the implemented one.
