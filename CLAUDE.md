# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```sh
just gen-mocks [dir]                # regenerate moq mocks (gitignored, incremental, hash-stamped)
just build [dir]                    # gen-mocks + go build
just test-short [dir]               # gen-mocks + unit tests (testcontainers tests skipped)
just test [dir]                     # gen-mocks + all tests incl. Postgres testcontainers (needs Docker)
just lint [dir]                     # gen-mocks + golangci-lint (strict, must be 0 issues)
go test -short -run 'TestName' ./internal/syncer/  # single test
golangci-lint run --fix ./internal/api/            # lint + autofix one package
golangci-lint fmt                   # format (gofumpt + goimports via config)
./test/e2e.sh                       # end-to-end test — requires Docker + Go, builds server/agent images + memoryctl
just windmill-failed-count          # read-only: failed Windmill completed jobs by path
```

golangci-lint is **v2** (config `version: "2"` in `.golangci.yml`). The VS Code and Claude hook autofix layers both run this same binary/config — do not introduce a second formatter or lint config.

## Architecture

Hub-and-spoke sync of Claude Code memory files (`~/.claude` content) across machines, **git-native since v2** (design record: `.docs/08-git-native-sync.md`). This repo is **code only** — memory content never lives here. Four Go binaries + a Windmill-hosted synthesis pipeline, all containerized (`build/*.Dockerfile`, `deploy/server/compose.yaml`):

- **`cmd/server`** — hub on the always-on home box. Hosts bare git repos under `<MEMORY_STORAGE_DIR>/repos/`: `canonical.git` (knowledge base, fetch-only over HTTP — receive-pack 403s at route level) and `clients/<id>.git` (one per machine, auto-created on first push with a pre-receive hook installed). `internal/gitserver` implements git smart-HTTP in Go (pkt-line advertisement + `git upload-pack|receive-pack --stateless-rpc`, gzip bodies, streaming); `internal/githook` is the pre-receive validation (branch must be `main`, path whitelist, blob modes 100644/100755 only, secret-pattern reject — whole push atomic), invoked as `server -githook-pre-receive` by the hook script. `internal/api` is now only healthz (huma v2) + bearer middleware wrapping both `/v1/` and `/git/`.
- **`cmd/agent`** — per-machine Docker agent. `internal/syncer.Agent.RunCycle` on a ticker: `scanLocal` whitelists files under `~/.claude` (mounted at `/claude`); **up-sync** rebuilds a local staging git repo (`<state>/staging`) to mirror the scan, commits and pushes to `clients/<id>.git` (auth via `http.extraHeader` bearer config); **down-sync** fetch+resets a canonical mirror clone (`<state>/canonical-mirror`), then 3-way diff (local vs saved `canonical.json` base vs mirror) applies changes locally — `BothChanged` = local wins, skip + warn.
- **`cmd/memoryctl`** — CLI with all synthesis-pipeline mechanics, baked into the Windmill worker image (`build/windmill-worker.Dockerfile`): `clients-scan`, `config-merge`, `ops-apply` (parses the `===FILE:/===DELETE:/===PROPOSAL:/NOTHING` op contract from stdin; guardrails via `internal/guardrails`; proposal targets whitelisted to `global/**` and `projects/<key>/memory/*.md`), `proposal-apply`, `proposal list|get|rejections|decide|revise|retry|apply-approved|expire|import` (pipeline-api client; `apply-approved` claims, writes+commits under the lock, pushes, and marks applied only after a successful push), `ops-apply --source-flow --source-run` (posts `===PROPOSAL:` ops to pipeline-api grouped by `GroupOf`), `commit`/`push` (flock on `/data/state/canonical.lock`), `markers-advance`, `hotcache-write`, `context`, `corpus list|read|search|digest`.
- **`cmd/pipeline`** — `pipeline-api`, the pipeline-state service (design: `.docs/11-pipeline-state-db.md`). huma v2 API on 8090 over its own Postgres 18 (`pipeline-db`), both on internal compose networks; only the API holds DB credentials. Holds proposals (status machine `pending→approved→applying→applied|failed`, `rejected`, `expired`, every change an event row), their files until applied, and rejections. `internal/pipeline` is the domain (types, `CanTransition`, `KindOf`/`GroupOf`/`PathspecOf`, `ValidateTarget` = the canonical proposal whitelist, `Repository` interface + `Service`); `internal/pipeline/postgres` the repository (pgx + sqlx named params, embedded goose migrations run at startup, advisory-lock per target, `FOR UPDATE SKIP LOCKED` lease claim); `internal/pipeline/httpapi` the API (writer vs admin bearer token per operation, single error-log boundary); `internal/pipeline/client` the typed client used by `memoryctl proposal …`. Workers get `PIPELINE_URL` + `PIPELINE_TOKEN` (writer on llm-steps, admin on ui-backend).
- **Windmill pipeline** (`deploy/windmill/`) — replaces the retired shell synthesizer services. Four flows (`flows/*.flow.yaml`, deployed to the Windmill instance on the server box, live paths `u/luciorosadini87/memory_*`): **intake** (45m: per-client diff since marker ref `refs/intake/last-processed`, mechanical last-writer-wins config merge, LLM novelty judgment per project), **daily-synth** (03:00: per-project condense/dedup, cross-project distillation posting proposals, hot-cache digest, push), **taskmine** (weekly: mines the corpus for proposals classified as `skill|rule|code-change|bug|one-time-task`; skills/rules target `global/**`, the rest become per-project memory entries), **proposal-apply** (10m + started by the app after approve/retry: expire, `apply-approved`, post-skill evidence cleanup, report). No flow suspends: proposals are posted to pipeline-api and approved in the app. Flow steps rely on a single `default` worker (`ensure-work` safety, see setup.md §8). Prompt text and parameters live in Windmill variables (`f/memory/*`, defaults in `prompts/*.md`) bound via static `$var:` input transforms. All LLM calls are `claude -p` with subscription auth (`CLAUDE_CODE_OAUTH_TOKEN`); every step refuses to run with `ANTHROPIC_API_KEY` set. One git commit per logical change (post-hoc audit). `deploy/windmill/app/` is a full-code React approval-inbox app over pipeline-api proposals (runnables shell out to `memoryctl proposal …` on the ui-backend worker; approve/reject/retry per proposal, event history, per-proposal LLM chat with revision apply); its own lint toolchain lives in that directory (`npm run lint`, `tsc --noEmit`, backend runnables gated by `mypy --strict` + ruff). Setup runbook: `deploy/windmill/setup.md`.
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
- No doc comments required (revive `exported` intentionally off) — repo convention is no comments at all.
- Mock dependencies with `moq`-generated mocks of consumer-side interfaces (`//go:generate go tool moq`; mocks are generated artifacts, gitignored, never committed). Git-touching code is tested against real git repos in `t.TempDir()` (`file://` bare remotes for the syncer, `httptest` + real `git clone`/`push` subprocesses for gitserver).
- gosec G204/G7xx exec findings are excluded by path for `internal/gitutil/gitutil.go` and `internal/gitserver/protocol.go` only — keep exec calls confined there.
- Secret patterns have a single source: Go (`internal/githook/secrets.go`, reused by `internal/guardrails`) — the old shell duplicate died with `build/synthesizer/`.

## Ground rules (from README — apply to all phases)

- Sync is **whitelist, not blacklist**: only explicitly listed paths leave `~/.claude`. Never sync `sessions/`, `ide/`, `daemon/`, `.credentials.json`, `cache/`, `paste-cache/`, `telemetry/`, `history.jsonl`, `shell-snapshots/`, `jobs/`, `tasks/`, `file-history/`.
- GitHub is backup only (server-side daily push), never the sync path.
- Machine list is open-ended — never hardcode a fixed set of hosts.
- Phase docs live in `.docs/` (00–12); phase 8 (git-native sync, `.docs/08-git-native-sync.md`) is the current sync architecture, phase 11 (`.docs/11-pipeline-state-db.md`) the approval/proposal state design, phase 12 the pending windmill-db Postgres 18 migration runbook — phase 1 is superseded history. Migration runbook for the old single-repo `/data` layout lives in the phase 8 doc.
