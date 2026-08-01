# Phase 8 — git-native sync (v2 architecture)

## Decision record (2026-08-01)

The phase-1 REST design (per-file PUT/DELETE + server-side dual-write
mirror into `canonical/`) is replaced by native git transport. User
requirements driving the change:

1. Clients send their **complete status** as real git commits — the
   agent keeps a local staging repo and "commits and pushes the
   difference".
2. The server derives what changed via **git diff** (native), not a
   hand-rolled manifest protocol.
3. What flows into the knowledge base (`canonical`) is decided by an
   **async intake pass**, not an instant mechanical mirror. LLM
   judgment applies only to `projects/*/memory/`; config trees
   (`global/CLAUDE.md`, `rules/`, `skills/`, `agents/`) merge
   mechanically (last-writer-wins by commit timestamp).
4. Down-sync stays **pull-based polling** (agent ticker fetches
   canonical).

Transport options weighed: evolving the REST API with an atomic batch
endpoint vs real git push. Git chosen — long-term structure prioritized;
transport, diffing, history, atomicity, resume and delta compression are
solved by git instead of maintained as custom protocol code.

## Storage layout (server volume)

```text
/data/repos/canonical.git          bare, branch main — the ONLY repo agents fetch
/data/repos/clients/<id>.git       bare, branch main — auto-created on first push,
                                   pre-receive hook installed
/data/work/canonical/              working clone; intake.sh + synth.sh edit, commit,
                                   push to canonical.git
/data/state/canonical.lock         flock serializing all canonical writes
/data/legacy-storage-backup/       pre-migration single-repo layout, kept for a
                                   safety window
```

Agent state volume: `staging/` (local git repo = the sync payload),
`canonical-mirror/` (clone of canonical.git), `canonical.json` (kept —
3-way down-sync base). `client.json` retired: git history is the
up-sync base now.

## Transport

- Smart-HTTP implemented directly in Go (`internal/gitserver`): pkt-line
  service advertisement + `git upload-pack|receive-pack --stateless-rpc`.
  No `git-http-backend` (absent from Alpine git), no CGI.
- Routes under `/git/`, wrapped by the same static bearer middleware as
  the REST API. Agents send the token via
  `git config http.extraHeader "Authorization: Bearer <token>"`.
- `canonical.git` is **never pushable over HTTP** — receive-pack against
  it returns 403 at the route level. Intake/synthesizer push to it
  locally over the shared volume.
- Trust boundary unchanged from v1: one shared token, client-repo
  isolation is nominal (any token holder could push to any client repo).
  Per-client tokens remain out of scope.

## Push validation (`internal/githook`)

Pre-receive hook in every client bare repo execs the server binary
(`server -githook-pre-receive`). Whole push rejected atomically if any
ref update violates:

- branch must be `refs/heads/main`, no deletion;
- every changed path matches the sync whitelist
  (`global/CLAUDE.md`, `global/rules/*.md`, `global/skills/**`,
  `global/agents/**`, `projects/*/memory/**`);
- blob modes only 100644/100755 (no symlinks/submodules);
- no secret-pattern match in added/modified blobs (same pattern family
  as the synthesizer's reject gate).

## Agent cycle

scan whitelist (unchanged `scan.go`/`slug.go`/`localpath.go`) →
rebuild staging worktree to mirror the scan → `git add -A`, commit if
staged, push → fetch canonical mirror →
`manifest.Diff(local, canonicalBase, canonicalCurrent)` → apply
RemoteOnlyChange/RemoteDelete, `BothChanged` = local wins (skip + warn)
→ save new canonical base.

## Intake pass (async, replaces dual-write)

`build/synthesizer/intake.sh`, own compose service (`memory-intake`,
same synthesizer image, `INTAKE_INTERVAL` default 45m):

- Per client repo: marker ref `refs/intake/last-processed`; skip when
  marker == head (cheap no-op). Changes =
  `git diff --name-status <marker>..main`.
- Config trees: mechanical last-writer-wins across clients by newest
  touching-commit author date.
- Project memory: LLM pass (`claude -p`, subscription OAuth token only)
  judges candidates against existing canonical entries — outputs only
  genuinely new/updated entries (`===FILE:` contract, no deletes;
  deletion authority stays with the daily condense pass). Same
  guardrails: kebab-case filenames, MEMORY.md never a target, secret
  reject.
- One commit per touched project + one for config, pushed to
  canonical.git under the shared flock.
- Marker advances only after the client's whole range processed; LLM
  failure leaves it in place — full range retried next pass (config
  reapply is idempotent).

Cross-machine propagation latency = client interval + intake interval +
client interval (worst case ~1h15m with defaults). Accepted: knowledge
sync does not need to be immediate.

## Migration runbook (one-off, maintenance window)

1. Stop old server + all agents.
2. `mkdir -p /data/repos /data/work /data/state`
3. `git init /data/work/canonical --initial-branch=main`; set
   `user.name`/`user.email` = memory-synthesizer.
4. Copy old `/data/canonical/*` (minus `.gitkeep`) into
   `/data/work/canonical/`; `git add -A`;
   `git commit -m "migrate: import canonical content from single-repo storage"`.
5. `git init --bare --initial-branch=main /data/repos/canonical.git`;
   add as `origin` of the work clone; `git push origin main`.
6. `mkdir /data/legacy-storage-backup && mv /data/canonical /data/clients /data/.git /data/legacy-storage-backup/`
7. Client repos start fresh — each machine's first push re-uploads its
   whole whitelist from live `~/.claude` (per-machine source of truth).
8. Deploy new server + intake + synthesizer images; upgrade **all**
   agents in the same window (old and new agents do not interoperate —
   synchronized cutover, no bridge).
9. Run each agent once manually (`MEMORY_RUN_ONCE=true`) to verify
   push/fetch before re-enabling tickers.

## Supersedes

- docs/01 REST API + single working repo + dual-write (kept as history).
- docs/03 Layer A "structural merge = dual-write" — structural merge is
  now the intake pass.
