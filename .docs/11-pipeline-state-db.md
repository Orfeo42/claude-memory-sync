# Phase 11 — Pipeline state service

Status: **decided 2026-10-11, phase 1 in implementation** (service, DB, client, deploy wiring). Phase 2 (flows, approval app, import of existing staging dirs) follows.

## Problem (incident 2026-10-11)

Human-gated state lives inside Windmill flow runs. `daily-synth` and `taskmine` stage proposals under `/data/state/proposals/<root-flow-job-id>/`, then park in a `suspend` step (`timeout: 604800`, 7 days) until someone resumes them.

- Runs never finish. A healthy pipeline shows a wall of "pending" runs, so stuck and waiting look the same.
- Deleting or canceling a run destroys the approval. Happened on 2026-08-30 (stale-run cleanup canceled suspended children) and on 2026-10-11 (bulk delete of failed runs also matched suspended jobs).
- In `daily-synth`, the approval gate (`promote_if_approved`) sits **before** `hotcache` and `push`: one pending approval holds that day's hot-cache digest for up to 7 days.
- The approval inbox reverse-engineers Windmill internals: `list_suspended.py` (226 lines) walks suspended flow state and parses the gate step's markdown result back into items.
- Expiry is a hidden side effect: `find /data/state/proposals -mtime +14 -exec rm -rf` at the start of every run.
- Rejections are a flat file (`/data/state/taskmine-rejected`) rewritten with `sort -u` + `mv`.

## Decision

Pipeline state moves out of Windmill into a dedicated service, `pipeline-api` (`cmd/pipeline`), backed by its own Postgres, `pipeline-db`. Every flow run terminates. Each stage reads work items in one status and moves them to the next, so new stages or branches are added by adding a status and a flow that consumes it.

Git (`canonical.git`) stays the only source of truth for memory content. The service holds only pipeline state: proposals, their files until applied, status history and rejections.

## Isolation

| network | members | purpose |
| --- | --- | --- |
| `pipeline-db-net` (`internal: true`) | `pipeline-db`, `pipeline-api` | only the API holds DB credentials |
| `pipeline` (`internal: true`) | `pipeline-api`, `windmill-worker-llm-steps`, `windmill-worker-ui-backend` | workers reach the API with a bearer token |

- `pipeline-db`: `postgres:18-alpine`, own volume `pipeline-db-data` mounted at `/var/lib/postgresql` (Postgres 18 image layout), own credentials (`PIPELINE_DB_PASSWORD`). Never shares the `windmill-db` instance, database or role. No published port.
- `pipeline-api`: own image `ghcr.io/orfeo42/claude-memory-sync-pipeline`, no published port, no `/data` mount. It is the only schema owner: migrations run at startup.
- Two tokens: `PIPELINE_TOKEN` (writer: create, list, read, claim, mark applied/failed, expire, rejections) and `PIPELINE_ADMIN_TOKEN` (writer + decide, revise, retry). The client always reads `PIPELINE_TOKEN`; compose sets it to the writer token on `windmill-worker-llm-steps` and to the admin token on `windmill-worker-ui-backend`. The LLM-steps worker, which runs `claude -p` over corpus content, cannot approve anything.
- Cost on the Pi: one Postgres process (~20–40 MB RAM idle) and one static Go process (a few MB).

## Data model

Migrations are embedded in `pipeline-api` (goose v3, SQL files) and idempotent both ways (`IF NOT EXISTS` / `IF EXISTS` on every DDL). Ids come from `gen_random_uuid()`.

```sql
CREATE TABLE IF NOT EXISTS proposal (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    source_flow   text NOT NULL,
    source_run    text NOT NULL,
    kind          text NOT NULL,
    name          text NOT NULL,
    status        text NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now(),
    decided_at    timestamptz,
    decided_by    text,
    claimed_by    text,
    claimed_until timestamptz,
    applied_sha   text,
    error         text,
    CONSTRAINT proposal_kind_chk CHECK (kind IN ('skill','rule','agent','global','project-entry','bug','one-time-task','code-change')),
    CONSTRAINT proposal_status_chk CHECK (status IN ('pending','approved','applying','applied','rejected','expired','failed'))
);

CREATE TABLE IF NOT EXISTS proposal_file (
    proposal_id  uuid NOT NULL REFERENCES proposal(id) ON DELETE CASCADE,
    target_path  text NOT NULL,
    content      text NOT NULL,
    PRIMARY KEY (proposal_id, target_path)
);

CREATE TABLE IF NOT EXISTS proposal_event (
    id           bigserial PRIMARY KEY,
    proposal_id  uuid NOT NULL REFERENCES proposal(id) ON DELETE CASCADE,
    from_status  text,
    to_status    text NOT NULL,
    actor        text NOT NULL,
    note         text,
    at           timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS rejection (
    kind         text NOT NULL,
    name         text NOT NULL,
    reason       text,
    rejected_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (kind, name)
);

CREATE INDEX IF NOT EXISTS proposal_status_created_idx ON proposal (status, created_at);
CREATE INDEX IF NOT EXISTS proposal_status_decided_idx ON proposal (status, decided_at);
CREATE INDEX IF NOT EXISTS proposal_status_claimed_idx ON proposal (status, claimed_until);
CREATE INDEX IF NOT EXISTS proposal_kind_status_idx ON proposal (kind, status);
CREATE INDEX IF NOT EXISTS proposal_file_target_idx ON proposal_file (target_path);
CREATE INDEX IF NOT EXISTS proposal_event_proposal_at_idx ON proposal_event (proposal_id, at);
```

No retention/cleanup task: rows are kept indefinitely; speed comes from indexes matching every hot query.

- `(status, created_at)`: inbox list, expiry scan. `(status, decided_at)`: claim order. `(status, claimed_until)`: expired-lease scan. `(kind, status)`: taskmine context and inbox filters. `proposal_file(target_path)`: open-proposal-per-target check. `proposal_event(proposal_id, at)`: history.
- A **proposal** is the unit a human approves: one skill (all files under `global/skills/<name>/`), one rule, one project entry. `kind` = `pipeline.KindOf(target)`, `name` = `pipeline.GroupOf(target)`; every file of a proposal must share the same group.
- Every target is validated by `pipeline.ValidateTarget` (the canonical whitelist) on create, on revise and again on apply, and secret-scanned (`internal/guardrails`).
- At most one open (`pending`/`approved`/`applying`) proposal per target path: create and revise take `pg_advisory_xact_lock(hashtext(target))` for every target (sorted) inside the transaction, then check, so concurrent creates cannot race.
- Every status change writes a `proposal_event` row in the same transaction. A rejection also upserts `rejection(kind, name)`.

## Status machine

```text
pending ──approve──▶ approved ──claim──▶ applying ──ok──▶ applied
   │                    ▲  ▲                 │
   │                    │  └──lease expired──┤
   │                    └──────retry─────────┴──err──▶ failed
   ├──reject──▶ rejected
   └──ttl─────▶ expired
```

- `pipeline.CanTransition(from, to)` is the single source of allowed edges.
- Each transition is one guarded `UPDATE … WHERE id = :id AND status = :from RETURNING …`; zero rows → `domain.ErrConflict` (or `domain.ErrNotFound` when the id does not exist).
- Field side effects: `approved`/`rejected` set `decided_at`/`decided_by`; `applying` sets `claimed_by`/`claimed_until`; leaving `applying` clears both; `applied` sets `applied_sha`; `failed` sets `error`; `approved` from `failed` clears `error`.
- Claim: one transaction resets every `applying` row with `claimed_until < now()` back to `approved` (event actor `lease-expired`), then takes the oldest `approved` by `decided_at` with `FOR UPDATE SKIP LOCKED` and moves it to `applying` with a lease (default 10 minutes).

## HTTP API (`pipeline-api`, huma v2, port 8090)

All bodies JSON; request-shape violations caught by huma annotations (enum, length, range, format) return 422, service-level validation (target whitelist, secrets, group mismatch) returns 400; errors are huma problem+json with `detail` = `domain.UserMessage(err)` and status from `domain.StatusCodeFromError`. `Proposal`, `File`, `Event`, `Rejection` are the `internal/pipeline` types (their `json` tags are the wire format).

| method + path | role | request | response |
| --- | --- | --- | --- |
| `GET /v1/healthz` | none | — | 200 |
| `POST /v1/proposals` | writer | `{source_flow, source_run, files:[{target_path, content}]}` | 201 `Proposal`; 400 invalid; 409 open proposal on a target |
| `GET /v1/proposals?status=&kind=&before=<RFC3339>&limit=` | writer | — | 200 `{proposals:[Proposal]}`, newest first, `limit` 1–500, default 100 |
| `GET /v1/proposals/{id}` | writer | — | 200 `{proposal: Proposal, events:[Event]}`; 404 |
| `POST /v1/proposals/{id}/decision` | admin | `{decision: "approve"\|"reject", actor, reason?}` | 200 `Proposal`; 409 not pending |
| `PUT /v1/proposals/{id}/files` | admin | `{actor, files:[File]}` | 200 `Proposal`; 409 not pending |
| `POST /v1/proposals/{id}/retry` | admin | `{actor}` | 200 `Proposal`; 409 not failed |
| `POST /v1/proposals/claim` | writer | `{worker, lease_seconds?}` | 200 `Proposal`; 204 nothing to claim |
| `POST /v1/proposals/{id}/applied` | writer | `{actor, sha}` | 200 `Proposal`; 409 not applying |
| `POST /v1/proposals/{id}/failed` | writer | `{actor, error}` | 200 `Proposal`; 409 not applying |
| `POST /v1/proposals/expire` | writer | `{actor, older_than_hours}` | 200 `{expired: n}` |
| `GET /v1/rejections?kind=` | writer | — | 200 `{rejections:[Rejection]}` |

Config (env, read in `cmd/pipeline/config.go`): `PIPELINE_DATABASE_URL` (required), `PIPELINE_TOKEN` (required), `PIPELINE_ADMIN_TOKEN` (required), `PIPELINE_PORT` (default `8090`).

## Packages

- `internal/pipeline` — domain: types, `Status`/`Kind` + parse, `CanTransition`, `KindOf`/`GroupOf`/`PathspecOf`, `ValidateTarget`, the `Repository` interface (moq mock), and `Service` (business rules on top of `Repository`).
- `internal/pipeline/postgres` — `Repository` implementation (pgx v5 stdlib + sqlx, named parameters only) and embedded goose migrations.
- `internal/pipeline/httpapi` — huma operations, IO wrappers, role middleware; the single error-log boundary.
- `internal/pipeline/client` — typed HTTP client + `ConfigFromEnv` (`PIPELINE_URL`, `PIPELINE_TOKEN`), maps HTTP status back to `domain` sentinels.
- `cmd/pipeline` — `pipeline-api` binary.
- `cmd/memoryctl` — `proposal` subcommands on top of the client; `ops-apply` posts proposals to the API.

## memoryctl surface

- `ops-apply … --source-flow <f> --source-run <r>` — `===PROPOSAL:` ops are grouped by `GroupOf` and each group is posted as one proposal; a 409 or 400 becomes a rejected op in the result, not a command failure. `--proposals-dir` stays until phase 2 switches the flows.
- `proposal list [--status] [--kind] [--limit]`, `proposal get --id`, `proposal rejections [--kind]` — JSON to stdout.
- `proposal decide --id --decision approve|reject --actor [--reason]`, `proposal revise --id --actor` (files JSON on stdin), `proposal retry --id --actor` — need the admin token.
- `proposal apply-approved --work-dir --lock --worker` — claim loop: write files (validated, secret-scanned, confined to the work dir), commit with pathspec `PathspecOf(target)` and message `proposal: apply <kind> <name>`, mark `applied` with `HEAD` sha or `failed` with the error; push once at the end if anything was applied. Prints `{applied:[ids], failed:[ids]}`; exits non-zero if any failed.
- `proposal expire --older-than <duration> --actor`.
- `proposal import --dir <staging-dir> --source-flow <f>` — one-off migration of existing staging dirs (flattened files + `.path` sidecars), `source_run` = the dir's basename.

## Testing

- The repository is a consumer-side interface with a `moq`-generated mock. Every test that is not about the database itself (service rules, transitions, HTTP handlers, client, memoryctl, stress, size and smoke) runs against the mock or an `httptest` server.
- Repository integration tests (SQL, migrations, guarded transitions, claim/lease concurrency, advisory-lock races, indexes) run against a real `postgres:18-alpine` via `testcontainers-go`, skipped under `-short`.
- Mock generation mirrors billing-backend: `moq` pinned in the `go.mod` `tool` block, `//go:generate go tool moq -out <x>_mock.go . <IFace>:Mock<IFace>` beside each interface, `**/*_mock.go` gitignored and never committed. `just gen-mocks [dir]` (incremental per package, `// mock-source-sum:` hash stamp, orphan removal) and `just gen-mocks-force`; `build`, `test`, `lint` recipes depend on `gen-mocks`.

## Phase 2

- Additions to the API: `POST /v1/rejections` (admin) `{kind, name, actor, reason?}` → 204, upsert into `rejection`; client `AddRejection`; `memoryctl proposal import-rejections --file --actor` (legacy one-name-per-line file; kind = `skill` when the name has no `.md` suffix, else `KindOf(name)`).
- `apply-approved` output gains `applied_proposals: [{id, kind, name, targets}]` (only proposals marked applied after a successful push), used by the cleanup step.
- `taskmine` = `ensure_work` → `draft`. The draft prompt's "pending" list is the names of `pending` + `approved` proposals and its "rejected" list comes from `memoryctl proposal rejections`; output goes to `ops-apply --source-flow taskmine --source-run <root job id>`. No approval branch.
- `daily-synth` = `ensure_work` → condense → distill (`ops-apply --source-flow daily-synth`) → `hotcache` → `push`, every day, with no gate in between.
- `proposal-apply` (new, Windmill path `f/memory/proposal_apply`, schedule `0 */10 * * * *` with `no_flow_overlap: true`, also started by the app after approve/retry) = `ensure_work` → `expire` (336h) → `apply` (`apply-approved`, exit code captured into the result) → `cleanup` (the former taskmine post-promotion LLM cleanup of evidence entries, for applied skills) → `report` (fails the run if anything failed).
- `ensure-work --lock` (shipped before phase 2): sync runs under `canonical.lock`; `HEAD` behind/equal → `reset --hard origin/main`; ahead → unpushed commits kept, only uncommitted junk dropped; diverged → `ErrConflict`, no reset. Safe because `canonical.git` only advances via pushes from this work dir and flow steps run one at a time on the single `default` worker; scaling that worker group beyond one would need per-step write locking.
- App runnables: `list_proposals`, `get_proposal`, `decide_proposal`, `retry_proposal`, `chat_proposal`, `apply_revision`, all through `memoryctl proposal …` (admin token on the `ui-backend` worker). `list_suspended`, `resume_run`, `cancel_run`, `debug_suspended` are deleted.

### Rollout (order matters)

1. Deploy phase 1 + phase 2 images: set `PIPELINE_DB_PASSWORD`, `PIPELINE_TOKEN`, `PIPELINE_ADMIN_TOKEN` (different values) in the Pi `.env`, sync `compose.yaml`, `docker compose pull && docker compose up -d`.
2. Old-shape runs still suspended at re-import time (none as of 2026-10-11) are canceled after step 4: their staged dirs are what step 4 imports, and canceling does not delete staged files.
3. Re-import `taskmine`, `daily-synth`, `intake` (only the `--lock` change) and import `proposal-apply`; attach its schedule.
4. One-off import inside `windmill-worker-ui-backend` (admin token): `memoryctl proposal import --dir /data/state/proposals/<id> --source-flow legacy-import` per dir, then `memoryctl proposal import-rejections --file /data/state/taskmine-rejected --actor legacy-import`.
5. Replace the app runnables and frontend (APP.md).
6. After one clean cycle: delete `/data/state/proposals/` and `/data/state/taskmine-rejected`.

## Decisions (2026-10-11)

1. Access: dedicated `pipeline-api` service; nothing else holds SQL or DB credentials.
2. Tests: `moq` mocks for everything not about the DB; `testcontainers-go` for repository integration tests.
3. Backup: none for `pipeline-db` for now; it holds only pipeline state.
4. Retention: no cleanup task; indexes carry query speed.
