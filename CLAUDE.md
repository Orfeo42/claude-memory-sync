# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```sh
go build ./...                      # build everything
go test -short ./...                # unit tests (all packages)
go test -short -run 'TestName' ./internal/syncer/  # single test
golangci-lint run ./...             # lint (strict, must be 0 issues)
golangci-lint run --fix ./internal/api/            # lint + autofix one package
golangci-lint fmt                   # format (gofumpt + goimports via config)
sh test/intake_smoke.sh             # intake/synth shell pipeline smoke test (stubbed claude, no Docker)
./test/e2e.sh                       # end-to-end test — requires Docker, builds all three images
```

golangci-lint is **v2** (config `version: "2"` in `.golangci.yml`). The VS Code and Claude hook autofix layers both run this same binary/config — do not introduce a second formatter or lint config.

## Architecture

Hub-and-spoke sync of Claude Code memory files (`~/.claude` content) across machines, **git-native since v2** (design record: `.docs/08-git-native-sync.md`). This repo is **code only** — memory content never lives here. Two Go binaries + shell services, all containerized (`build/*.Dockerfile`, `deploy/*/compose.yaml`):

- **`cmd/server`** — hub on the always-on home box. Hosts bare git repos under `<MEMORY_STORAGE_DIR>/repos/`: `canonical.git` (knowledge base, fetch-only over HTTP — receive-pack 403s at route level) and `clients/<id>.git` (one per machine, auto-created on first push with a pre-receive hook installed). `internal/gitserver` implements git smart-HTTP in Go (pkt-line advertisement + `git upload-pack|receive-pack --stateless-rpc`, gzip bodies, streaming); `internal/githook` is the pre-receive validation (branch must be `main`, path whitelist, blob modes 100644/100755 only, secret-pattern reject — whole push atomic), invoked as `server -githook-pre-receive` by the hook script. `internal/api` is now only healthz (huma v2) + bearer middleware wrapping both `/v1/` and `/git/`.
- **`cmd/agent`** — per-machine Docker agent. `internal/syncer.Agent.RunCycle` on a ticker: `scanLocal` whitelists files under `~/.claude` (mounted at `/claude`); **up-sync** rebuilds a local staging git repo (`<state>/staging`) to mirror the scan, commits and pushes to `clients/<id>.git` (auth via `http.extraHeader` bearer config); **down-sync** fetch+resets a canonical mirror clone (`<state>/canonical-mirror`), then 3-way diff (local vs saved `canonical.json` base vs mirror) applies changes locally — `BothChanged` = local wins, skip + warn.
- **`build/synthesizer/`** — shell + Claude Code CLI services on the server volume, sharing `lib.sh` guardrails (op parsing, kebab-case filename validation, secret reject, MEMORY.md rebuild) and a `flock` on `/data/state/canonical.lock`: **`intake.sh`** (own compose service `memory-intake`, `INTAKE_INTERVAL` default 45m) reads new commits per client repo since marker ref `refs/intake/last-processed`, merges config trees mechanically (last-writer-wins by commit timestamp) and passes changed `projects/*/memory/` entries through an LLM novelty judgment before committing to canonical; **`synth.sh`** (daily) condenses/dedups the standing canonical corpus. Both edit the `/data/work/canonical` clone and push to `canonical.git`. Subscription auth via `CLAUDE_CODE_OAUTH_TOKEN` (`claude setup-token`); refuse to start with `ANTHROPIC_API_KEY` set. One git commit per logical change (post-hoc audit).
- **`internal/manifest`** — content-hash manifests + `manifest.Diff`, the 3-way diff engine down-sync relies on. Change kinds (`RemoteOnlyChange`, `RemoteDelete`, `BothChanged`, …) drive the apply step.
- **`internal/gitutil`** — shared git shell-out helpers (`Run`, `RunOutput`, `HasStagedChanges`); the only place allowed to exec git.
- **`internal/syncer/slug.go` / `localpath.go`** — mapping between local `~/.claude` paths and namespaced sync paths (slug prefix per machine identifies per-project memory dirs).

Sync whitelist (`internal/syncer/scan.go`): `CLAUDE.md` → `global/CLAUDE.md`, `rules/*.md` → `global/rules/`, `skills/` and `agents/` full trees → `global/skills/` / `global/agents/` (phase 2a), `projects/<slug>/memory/` → `projects/<canonical-key>/memory/`.

### Error handling (`internal/domain`)

Ported pseudo-stack design; all new error paths must use it:

- Sentinels are **constants**: `type sentinelError string` with `const ErrNotFound/ErrUnauthorized/ErrValidation/ErrConflict/ErrUpstream/ErrInternal`.
- Wrap with `domain.Error(err, msg, attrs...)` / `domain.WrapError(err, attrs...)` — nil-safe, auto-appends `origin` (`file:line`) attr. Structured context (namespace, path, client_id) travels **on the error**, not in intermediate logs.
- Log **once at the boundary** only: server boundary in `internal/gitserver/handler.go`, agent boundary in `cmd/agent` `runCycle`. `domain.LogError` / `domain.LogWarn` flatten `domain.ErrorAttrs` (origins as pseudo-stack) into the record. Never log-then-return mid-chain. (`internal/githook.Validate` returns errors, never logs — its stderr is the hook protocol.)
- Level by fault source: upstream/dependency fault (`errors.Is(err, domain.ErrUpstream)`, `UpstreamError`, `UpstreamUnreachableError`) → Warn; own fault → Error; expected 4xx → not logged.
- HTTP mapping via `domain.StatusCodeFromError` + `domain.UserMessage` — upstream 5xx surfaces as 502, never blanket 500.

## Lint/style constraints baked into `.golangci.yml`

- sloglint: snake_case keys, static messages, `*Context` logger variants required, typed attrs.
- depguard bans `log` and `pkg/errors`; slog only.
- gofumpt needs `module-path: claude-memory-sync` in config (already set) — removing it causes phantom "not properly formatted" findings.
- No doc comments required (revive `exported` intentionally off) — repo convention is no comments at all (two allowed exceptions: the "keep in sync" cross-references between the Go and shell secret patterns).
- No mocks: git-touching code is tested against real git repos in `t.TempDir()` (`file://` bare remotes for the syncer, `httptest` + real `git clone`/`push` subprocesses for gitserver).
- gosec G204/G7xx exec findings are excluded by path for `internal/gitutil/gitutil.go` and `internal/gitserver/protocol.go` only — keep exec calls confined there.
- Secret patterns exist twice by design: Go (`internal/githook/secrets.go`) and shell (`build/synthesizer/lib.sh` `secret_pattern`) — update both together.

## Ground rules (from README — apply to all phases)

- Sync is **whitelist, not blacklist**: only explicitly listed paths leave `~/.claude`. Never sync `sessions/`, `ide/`, `daemon/`, `.credentials.json`, `cache/`, `paste-cache/`, `telemetry/`, `history.jsonl`, `shell-snapshots/`, `jobs/`, `tasks/`, `file-history/`.
- GitHub is backup only (server-side daily push), never the sync path.
- Machine list is open-ended — never hardcode a fixed set of hosts.
- Phase docs live in `.docs/` (00–08); phase 8 (git-native sync, `.docs/08-git-native-sync.md`) is the current architecture — phase 1 is superseded history. Migration runbook for the old single-repo `/data` layout lives in the phase 8 doc.
