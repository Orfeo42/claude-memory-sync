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
just web-check                      # web/ SPA: eslint, tsc, vitest, build
```

golangci-lint is **v2** (config `version: "2"` in `.golangci.yml`). The VS Code and Claude hook autofix layers both run this same binary/config — do not introduce a second formatter or lint config.

## Architecture

Hub-and-spoke sync of Claude Code memory files (`~/.claude` content) across machines, **git-native since v2** (design record: `.docs/08-git-native-sync.md`). This repo is **code only** — memory content never lives here. Five Go binaries (server, agent, memoryctl, pipeline-api, synth) plus a React SPA, containerized except `memoryctl` (`build/*.Dockerfile`, `deploy/server/compose.yaml`):

- **`cmd/server`** — hub on the always-on home box. Hosts bare git repos under `<MEMORY_STORAGE_DIR>/repos/`: `canonical.git` (knowledge base, fetch-only over HTTP — receive-pack 403s at route level) and `clients/<id>.git` (one per machine, auto-created on first push with a pre-receive hook installed). `internal/gitserver` implements git smart-HTTP in Go (pkt-line advertisement + `git upload-pack|receive-pack --stateless-rpc`, gzip bodies, streaming); `internal/githook` is the pre-receive validation (branch must be `main`, path whitelist, blob modes 100644/100755 only, secret-pattern reject — whole push atomic), invoked as `server -githook-pre-receive` by the hook script. `internal/api` is now only healthz (huma v2) + bearer middleware wrapping both `/v1/` and `/git/`.
- **`cmd/agent`** — per-machine Docker agent. `internal/syncer.Agent.RunCycle` on a ticker: `scanLocal` whitelists files under `~/.claude` (mounted at `/claude`); **up-sync** rebuilds a local staging git repo (`<state>/staging`) to mirror the scan, commits and pushes to `clients/<id>.git` (auth via `http.extraHeader` bearer config); **down-sync** fetch+resets a canonical mirror clone (`<state>/canonical-mirror`), then 3-way diff (local vs saved `canonical.json` base vs mirror) applies changes locally — `BothChanged` = local wins, skip + warn.
- **`cmd/memoryctl`** — thin CLI over the logic in `internal/{workrepo,intake,corpus,ops,proposalapply,hotcache}` (run by hand or from scripts; the synth service calls the same packages in-process). Subcommands: `ensure-work`, `clients-scan`, `config-merge`, `markers-advance` (intake mechanics), `context`, `corpus list|read|search|digest`, `ops-apply` (parses the `===FILE:/===DELETE:/===PROPOSAL:/NOTHING` op contract from stdin; guardrails via `internal/guardrails`; proposal targets whitelisted to `global/**` and `projects/<key>/memory/*.md`; `--source-flow --source-run` posts `===PROPOSAL:` ops to pipeline-api grouped by `GroupOf`), `commit`/`push` (flock on `/data/state/canonical.lock`), `hotcache-write`, `proposal-apply`, `proposal list|get|rejections|decide|revise|retry|apply-approved|expire|import|import-rejections` (pipeline-api client; `apply-approved` claims, writes+commits under the lock, pushes, and marks applied only after a successful push).
- **`cmd/pipeline`** — `pipeline-api`, the pipeline-state service (design: `.docs/11-pipeline-state-db.md`). huma v2 API on 8090 over its own Postgres 18 (`pipeline-db`), both on internal compose networks; only the API holds DB credentials. Holds three domains, each with its own tables, migrations and routes: proposals (status machine `pending→approved→applying→applied|failed`, `rejected`, `expired`, every change an event row), their files until applied, and rejections), settings (`internal/settings`: key/value with append-only versions; `secret.*` keys keep no history and are returned only to the writer/admin token) and runs (`internal/runs`: runs + steps, interrupted marking, retention prune). `internal/pipeline` is the domain (types, `CanTransition`, `KindOf`/`GroupOf`/`PathspecOf`, `ValidateTarget` = the canonical proposal whitelist, `Repository` interface + `Service`); `internal/pipeline/postgres` the repository (pgx + sqlx named params, embedded goose migrations run at startup, advisory-lock per target, `FOR UPDATE SKIP LOCKED` lease claim); `internal/pipeline/httpapi` the API (writer vs admin bearer token per operation, single error-log boundary; the admin token also passes the writer routes; `httpapi.Registrar` lets each extra domain register its routes on the shared API); `internal/pipeline/client` the typed client used by `memoryctl` and synth. Clients get `PIPELINE_URL` + `PIPELINE_TOKEN` (synth uses the admin token).
- **`cmd/synth`** — the synthesis service, replaces the retired Windmill pipeline (design: `.docs/13-drop-windmill.md`; cutover runbook in the same doc). One container with an in-process gocron scheduler (`internal/synthsched`; schedules stored as settings with defaults `*/45 * * * *` intake, `0 3 * * *` daily-synth, `0 4 * * 0` taskmine, `*/10 * * * *` proposal-apply; stale runs marked interrupted at startup; 90-day run prune), the flow runner `internal/flows` (four flows: **intake** per-client diff since `refs/intake/last-processed`, config merge, LLM novelty judgment per project; **daily-synth** per-project condense/dedup, cross-project distillation posting proposals, hot-cache digest, push; **taskmine** weekly corpus mining into proposals classified `skill|rule|code-change|bug|one-time-task`; **proposal-apply** expire, `apply-approved`, post-skill evidence cleanup, report), one run per flow at a time, the shared `canonical.lock` across flows, skip-failures semantics per flow (one failing project does not stop the rest; the run ends `partial`), one git commit per logical change, and a C1 fidelity gate: before a condense is committed a judge prompt (`fidelity_model`) lists facts lost, and any loss skips that project and records the step as `lossy`. `internal/llm` is the only package that execs `claude -p` (prompt on stdin), refuses to run with `ANTHROPIC_API_KEY` set, and takes the OAuth token from the settings DB first, then the `CLAUDE_CODE_OAUTH_TOKEN` env. The huma v2 API under `/api` (`internal/synthapi`: proposals, chat, revision, runs, run-now, prompts, settings, token) fronts pipeline-api so the admin token stays server-side; the SPA from `web/` is served from `SYNTH_WEB_DIR`. No auth: anyone who reaches the port can approve proposals into `global/**` and replace the token, so the compose port is bound to `127.0.0.1` by default (`SYNTH_BIND`). `internal/synthsettings` layers prompt overrides (with history and reset to the embedded defaults in `internal/flows/prompts/*.md`), params (`synth_model`, `fidelity_model`, `staleness_enabled`, `hotcache_max_bytes`), schedules and the write-only token over the pipeline-api settings domain. Run history lives in pipeline-api (`internal/runs`). `internal/footprint` computes the memory footprint report on demand (`/api/report/footprint`: per-project `MEMORY.md` lines/bytes against the harness limit, global CLAUDE.md and rules split into unconditional and path-scoped). `web/` is a Vite + React + TypeScript SPA (pages: proposals, runs, prompts, settings, footprint; `npm run lint|typecheck|test|build`). `internal/hiddentext` is the single source of the hidden-character guard (bidi controls, tag characters, zero-width characters, stray control characters), applied in the pre-receive hook, `ops-apply` and proposal create/revise.
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
- Phase docs live in `.docs/` (00–13); phase 8 (git-native sync, `.docs/08-git-native-sync.md`) is the current sync architecture, phase 11 (`.docs/11-pipeline-state-db.md`) the approval/proposal state design, phase 13 (`.docs/13-drop-windmill.md`) the current pipeline design (synth service) and Windmill cutover runbook — phases 9, 10 and 12 (Windmill) and phase 1 are superseded history. Migration runbook for the old single-repo `/data` layout lives in the phase 8 doc.
