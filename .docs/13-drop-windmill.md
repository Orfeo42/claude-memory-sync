# Phase 13 — Drop Windmill, adopt atlas ideas

Status: **implemented 2026-11-21 on `feat/drop-windmill`, cutover pending** (W8).

## Problem

Windmill hosts four cron schedules, bash glue between `memoryctl` and `claude -p`, one `forloop` with `skip_failures`, prompts as `$var:` variables and the approval inbox app. All real logic already lives in `memoryctl` and `pipeline-api`. The cost: four containers (server, Postgres 16, two workers), no IaC (the app's six Python runnables are pasted into the editor by hand), a Python/mypy/ruff toolchain used only by the app, the `BASE_INTERNAL_URL` hack to start the apply flow, and the pending windmill-db PG18 migration (phase 12).

Separately, the atlas project (`github.com/bloodfeast/atlas`, reviewed 2026-11-21) cuts session tokens with ideas that apply here: conditional rule loading, a hidden-character guard on injected text, and measuring memory fidelity and footprint instead of assuming it.

## Decision

### Topology

| Service | Change |
| --- | --- |
| `memory-server`, `memory-backup` | unchanged |
| `pipeline-db` | unchanged image, new per-domain migrations |
| `pipeline-api` | gains the `internal/settings` and `internal/runs` domains (own tables, own migrations, own routes); still the only holder of DB credentials |
| `synth` (new) | scheduler, flow runner, `claude` CLI, `/data` volume, serves the approval SPA and a JSON API that fronts pipeline-api (admin token stays server-side) |
| `windmill-db`, `windmill-server`, `windmill-worker-*` | removed, with `build/windmill-worker.Dockerfile`, `deploy/windmill/` and phase 12 |

### No auth on synth (for now)

The synth UI is a local tool. Anyone who reaches its port can approve proposals into `global/**` (synced to every machine and loaded into every session), replace the OAuth token and start LLM runs. Guards: the compose port binding is `${SYNTH_BIND:-127.0.0.1}:8082:8082`, so LAN exposure is an explicit `.env` choice; the API never returns the token value; every approve and every token change writes an event row.

### Phase A — atlas ideas, independent of the Windmill removal

- **A1 — rules loading.** Rule files without `paths:` frontmatter load at launch (code.claude.com/docs/en/memory). Language rules get `paths:` globs; topic rules become skills or get file globs, per file, reviewed by the user; duplicated rules are merged; the stale "Vertical rules — READ the file" section of the global CLAUDE.md is rewritten. The taskmine and distill-write prompts emit `paths:` for language-specific rules. Measured with `/context` before and after.
- **A2 — hidden-character guard.** `internal/hiddentext` is the single source. Rejected: bidi controls, tag characters, zero-width space / word joiner / Mongolian vowel separator, BOM past offset 0, C0 controls other than tab/LF/CR. Allowed in context only: ZWJ/ZWNJ between two non-ASCII characters, VS15/VS16 after a non-ASCII character or a keycap base. Wired into the pre-receive hook, ops-apply and proposal create/revise.

### Phase W — Windmill removal

- **W1** — `memoryctl` logic moves from `package main` into `internal/workrepo`, `internal/intake`, `internal/corpus`, `internal/ops` (apply) and `internal/proposalapply`, behaviour unchanged. `memoryctl` stays a thin CLI.
- **W2** — `internal/llm` is the only package that execs `claude` (prompt on stdin, `-p --model`), refuses to run with `ANTHROPIC_API_KEY` set, and takes the OAuth token from a token source at call time. Default prompts move to `internal/flows/prompts/*.md`, embedded.
- **W3** — `internal/settings` (key/value with append-only versions; `secret.*` keys keep no history and are returned only to the writer/admin token) and `internal/runs` (runs + steps, interrupted marking, 90-day retention) in pipeline-api, plus client methods.
- **W4** — `internal/flows`: Go port of intake, daily-synth, taskmine and proposal-apply, same semantics (skip-failures per flow, one run per flow, the existing `canonical.lock` across flows, one commit per logical change).
- **W5** — `cmd/synth`: in-process scheduler (schedules stored as settings, defaults `*/45 * * * *`, `0 3 * * *`, `0 4 * * 0`, `*/10 * * * *`), interrupted-run marking at startup, huma v2 API under `/api` (proposals, chat, revision, runs, run-now, prompts with history and reset, settings, write-only token), the SPA served from `SYNTH_WEB_DIR`.
- **W6** — `web/`: the approval SPA moved from `deploy/windmill/app`, talking to `/api`, plus Runs, Prompts and Settings pages. Vite, ESLint, tsc, Vitest + Testing Library.
- **W7** — `build/synth.Dockerfile` (amd64, arm64), compose, publish matrix, docs.
- **W8** — cutover on the server box: disable Windmill schedules, deploy synth, set the token, run each flow once, remove the Windmill services, then (irreversible) drop `windmill-db-data`.

### Phase C — fidelity and footprint

- **C1** — before a condense is committed, a judge prompt (`fidelity_model`, default haiku) compares old and new text and returns the facts lost. Any loss skips that project's condense and records the step as `lossy` with the list.
- **C2** — `/api/report/footprint`: per-project `MEMORY.md` lines and bytes against the harness limit (200 lines or 25 KB, whichever first), entry count, global CLAUDE.md and rules split into unconditional and path-scoped; daily-synth stores a snapshot per run; warning past 80% of the limit.

## Order

Wave 1 in parallel: A1, A2, W1, W2, W3. Then the contracts (`flows` consumer interfaces, `flows.Runner`, synth OpenAPI) are frozen. Wave 2 in parallel: W4, W5, W6. Then W7, C1, C2, W8.

## Out of scope

- Relevance-gated recall through a `UserPromptSubmit` hook with a slimmed `MEMORY.md` (per-machine component, own design doc).
- atlas's tool-output handles, code index and compaction cards (session-level, not memory sync).

## Frozen contract — synth HTTP API (W5 backend, W6 SPA)

All JSON. Base path `/api`; everything else serves the SPA (`SYNTH_WEB_DIR`, unknown non-`/api` paths fall back to `index.html`). No auth. Every write uses actor `ui`. Errors use huma's problem JSON (`{"title","status","detail"}`). Lists are wrapped objects, never bare arrays. Timestamps RFC 3339.

Types reused verbatim from pipeline-api: `Proposal`, `ProposalEvent`, `ProposalDetail {proposal, events}` (as in `deploy/windmill/app/types.ts` today), `Run`, `Step` (`internal/runs` json tags: run `id, flow, trigger, status, started_at, finished_at?, error?, summary`; step `id, run_id, step, status, started_at, finished_at, error?, output_tail?`).

| Method | Path | Request | Response |
| --- | --- | --- | --- |
| GET | `/api/proposals` | query `status` optional (one proposal status); omitted = pending + failed + approved + applying | `{proposals: Proposal[]}` newest first |
| GET | `/api/proposals/{id}` | | `ProposalDetail` |
| POST | `/api/proposals/{id}/decision` | `{decision: "approve" \| "reject", reason?: string}` | `{proposal: Proposal, apply_started: bool}`; approve starts proposal-apply in the background unless it is already running |
| POST | `/api/proposals/{id}/retry` | | `{proposal: Proposal, apply_started: bool}` |
| POST | `/api/proposals/{id}/chat` | `{message: string, model: "sonnet" \| "opus" \| "haiku", history: {role: "user" \| "assistant", content: string}[]}` | `{reply: string}` |
| POST | `/api/proposals/{id}/revision` | `{reply: string}` (an assistant reply holding `===FILE: <target>===` … `===END===` blocks) | `{applied: string[], proposal: Proposal}`; only targets already in the proposal are applied |
| GET | `/api/flows` | | `{flows: {name, schedule, default_schedule, running: bool, last_run: Run \| null}[]}` in order intake, daily-synth, taskmine, proposal-apply |
| POST | `/api/flows/{name}/run` | | 202 `{flow}`; 409 when that flow is running |
| GET | `/api/runs` | query `flow?`, `status?`, `limit?` (1–200, default 50), `before?` | `{runs: Run[]}` |
| GET | `/api/runs/{id}` | | `{run: Run, steps: Step[]}` |
| GET | `/api/prompts` | | `{prompts: {name, overridden: bool, updated_at: string \| null}[]}` sorted by name |
| GET | `/api/prompts/{name}` | | `{name, content, default, overridden, updated_at \| null}` |
| PUT | `/api/prompts/{name}` | `{content: string}` (1–200000 chars) | same as GET |
| DELETE | `/api/prompts/{name}` | | same as GET (reset to default) |
| GET | `/api/prompts/{name}/history` | query `limit?` | `{versions: {content: string \| null, deleted: bool, created_at, created_by}[]}` newest first |
| GET | `/api/settings` | | `{params: {key, value, default, kind: "model" \| "bool" \| "int", description, overridden}[], token: {set: bool, updated_at: string \| null}}` |
| PUT | `/api/settings/params/{key}` | `{value: string}` (validated by kind) | one param object |
| DELETE | `/api/settings/params/{key}` | | one param object (reset) |
| PUT | `/api/flows/{name}/schedule` | `{cron: string}` (5-field) | one flow object; scheduler reloads that flow |
| DELETE | `/api/flows/{name}/schedule` | | one flow object (reset to default) |
| PUT | `/api/settings/token` | `{token: string}` | `{set: true, updated_at}`; the value is never returned anywhere |
| DELETE | `/api/settings/token` | | `{set: bool, updated_at}` (falls back to the `CLAUDE_CODE_OAUTH_TOKEN` env bootstrap when set) |
| GET | `/api/report/footprint` | | `{generated_at, limits: {memory_lines: 200, memory_bytes: 25600, warn_pct: 80}, projects: {key, memory_lines, memory_bytes, entries, pct_of_limit}[], global: {claude_md_bytes, rules_unconditional_bytes, rules_scoped_bytes, rules_unconditional: string[], rules_scoped: string[], skills: int}}` |

Settings keys behind it (pipeline-api `settings` domain): `prompt.<name>`, `param.<key>`, `schedule.<flow>`, `secret.claude-oauth-token`. Params and defaults: `synth_model` (model, `sonnet`), `fidelity_model` (model, `haiku`), `staleness_enabled` (bool, `false`), `hotcache_max_bytes` (int, `4000`).

Container env: `SYNTH_ADDR` (default `:8082`; host exposure is decided by the compose port binding `${SYNTH_BIND:-127.0.0.1}:8082:8082`), `SYNTH_WEB_DIR` (`/srv/web`), `PIPELINE_URL`, `PIPELINE_TOKEN` (admin), `MEMORY_REPOS_DIR` (`/data/repos`), `MEMORY_WORK_DIR` (`/data/work/canonical`), `MEMORY_LOCK` (`/data/state/canonical.lock`), optional `CLAUDE_CODE_OAUTH_TOKEN` (bootstrap fallback when the DB holds no token).

## Deviations from the decision (as implemented)

- C2: no per-run snapshot. The footprint is computed on demand at `/api/report/footprint`, so daily-synth stores nothing extra.
- Revision rule follows the old app: a revision may touch the proposal's existing targets plus new files in the same skill group.
- Validation errors map to 422 in the synth API; an unknown flow name is a 404.
- daily-synth skips the fidelity judge when condense answers `NOTHING`.
- The proposal-apply worker id is `synth:<run id>`.
- `web/` uses TypeScript 6.0.3 because typescript-eslint 8.73 supports `<6.1`.

## Cutover (W8)

Run on the server box, in the directory that holds `compose.yaml` and `.env`. Service names below are those of the final compose file (`pipeline-api`, `synth`).

1. Windmill UI: disable the four schedules; wait until no Windmill job is running.
2. Update `compose.yaml` and `.env` on the server box, as for any deploy: prebuilt GHCR images only, never build on the box. Keep `WINDMILL_DB_PASSWORD` in `.env` until step 10.
3. `docker compose pull pipeline-api synth`
4. `docker compose up -d pipeline-api` (runs the new settings and runs migrations at startup; check `docker compose logs pipeline-api | grep migrated`).
5. Ownership fix (the Windmill workers ran as root, synth runs as uid 1000): `docker compose run --rm --no-deps --user root --entrypoint chown synth -R 1000:1000 /data/work /data/state`
6. `docker compose up -d synth`; `docker compose logs synth` shows `starting synth`.
7. Open the UI (`ssh -L 8082:127.0.0.1:8082 <server>`, then http://localhost:8082). Settings: set the Claude OAuth token (or rely on the `CLAUDE_CODE_OAUTH_TOKEN` env bootstrap). Check that Proposals shows the same pending proposals as before (state lives in pipeline-api, untouched).
8. Runs: "Run now" proposal-apply, then intake, then daily-synth (taskmine optional). Each must end `succeeded` or `partial` with readable steps. The Footprint page renders.
9. `docker compose up -d --remove-orphans` (removes the Windmill containers that are no longer in compose).
10. IRREVERSIBLE, only after an explicit decision: if Windmill run history is not wanted, no dump is needed; otherwise dump first. Find the exact volume name with `docker volume ls | grep windmill`, then `docker volume rm <project>_windmill-db-data`. Then remove `WINDMILL_DB_PASSWORD` from `.env`.

Rollback (before step 9): `docker compose stop synth`, restore the previous `compose.yaml` and `.env`, `docker compose up -d`, re-enable the Windmill schedules. The new pipeline-api tables are additive and harmless to the old flows.
