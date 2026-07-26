# Phase 1 — containerized hub sync (Go API server + Docker agents)

## Decision record — host-native design rejected (2026-07-19)

The first executed design (Syncthing P2P + bash `rsync` scripts + systemd
user timers) was rejected by the user after implementation started: it
hard-depends on Linux on every machine (systemd, bash, host packages).
Requirements replacing it:

- Fully containerized — the only per-machine dependency is Docker.
- Multiple containers, each one concern.
- **Central custom Go API server** on an always-on home box (chosen over
  GitHub-as-hub and over a plain self-hosted git remote — the server
  will also host the phase 3/5 synthesis jobs server-side).
- The "local-first / P2P / no central server" ground rule is dropped
  (README amended). GitHub returns to backup-only, pushed from the
  server, never a sync path.
- Git remains the storage core: server-side repo history is the
  phase 3/5 post-hoc audit trail.

## Architecture

### Server (home box, Docker Compose)

- `memory-server`: Go REST API. Storage = git repo on a named volume,
  layout unchanged: `global/`, `projects/<canonical-key>/memory/`.
- `memory-backup`: daily `git push` of the storage repo to GitHub
  (deploy key mounted read-only).
- Future: phases 3/5/6 containers live on this same box.

### Client (each machine, Docker Compose)

- `memory-agent`: Go binary, sync loop (default 15 min, plus one run at
  start). Bind mounts `~/.claude` rw; named volume for agent state.
- Env: `SERVER_URL`, `TOKEN`, `CLIENT_ID` (hostname-ish, unique per
  machine), `SLUG_PREFIX`, `INTERVAL`.
- Slug translation, cross-platform: `SLUG_PREFIX` (e.g. `-home-orfeo42`)
  maps the local slug prefix to canonical token `HOME` and back. A
  Windows machine just sets its own prefix (e.g. `C--Users-x`). Slugs
  not matching the prefix are skipped with a warning.
- Global files: the agent syncs `global/CLAUDE.md` and `global/rules/`
  directly into `~/.claude/` as real files. The phase 0 symlinks are
  reverted at rollout — symlinks were Linux-specific.

### Storage model — per-client namespaces + canonical tree (user requirement, 2026-07-19)

The server never merges client uploads into one shared tree. Each
client's memory is stored separately; nothing is ever overwritten by
another machine. The curated tree flows back only after condensation
(phases 3/5):

```text
storage repo (named volume, git)/
├── canonical/                 curated tree — the ONLY source clients pull
│   ├── global/                CLAUDE.md, rules/  (seeded from this repo at rollout)
│   └── projects/<key>/memory/
└── clients/<client-id>/       raw per-machine mirror — single writer: that client
    ├── global/
    └── projects/<key>/memory/
```

- Up-sync: agent mirrors its local state into `clients/<client-id>/…`
  only. Single writer per namespace → cross-client conflicts are
  structurally impossible; the conflict-file mechanism is gone.
- Down-sync: agents pull from `canonical/…` only. Phases 3/5 read all
  `clients/*` namespaces, condense/dedup, and write `canonical/` —
  that is the "sync back after cleanup" loop.
- Until phases 3/5 exist: `canonical/global/` is seeded at rollout by
  copying one machine's uploaded `clients/<id>/global/` into
  `canonical/global/` on the server (so global rules sync across
  machines day 1); `canonical/projects/` may start empty — per-project
  memory flows up and accumulates safely, flows back once synthesis
  lands.
- Repo split (user decision, 2026-07-19): this repo holds CODE ONLY.
  Memory content lives in the hub storage volume; its off-site backup
  is a separate private GitHub repo (`claude-memory`), pushed daily by
  the backup container.
- Git history on the whole storage repo = audit trail; deletions in a
  client namespace mirror that machine's local deletions and stay
  recoverable via git.

### API (v1, JSON, static bearer token)

- `GET  /v1/clients/{id}/tree` — manifest `[{path, sha256, size}]`.
- `PUT  /v1/clients/{id}/file/{path}` — write + immediate git commit
  (`sync: {id} {path}`) — one commit per change (phase 5 audit model).
- `DELETE /v1/clients/{id}/file/{path}` — same commit semantics.
- `GET  /v1/canonical/tree` / `GET /v1/canonical/file/{path}` —
  read-only for agents (canonical writes come from server-side
  synthesis jobs, not from this API version).
- `GET  /v1/healthz`.
- `{path}` is namespace-relative: `global/CLAUDE.md`,
  `projects/<key>/memory/<file>`.

### Sync algorithm

Agent persists two base manifests in its state volume (last-uploaded
client state, last-applied canonical state).

- **Up-sync** (local vs client-namespace base): changed/new locally →
  PUT; deleted locally → DELETE. Plain mirror, no merge logic.
- **Down-sync** (canonical vs canonical base vs local): canonical
  changed AND local unchanged since base → apply locally (incl.
  deletes). Canonical changed AND local also changed → **local wins**,
  skip + log — the local edit flows up to the client namespace and
  returns via the next condensation pass. Nothing is ever lost: every
  local version reaches `clients/<id>/` before anything could touch it.
- Base manifests updated at cycle end.

The per-file base fixes the deletion-resurrection flaw the rsync design
had (a deletion is distinguishable from a never-had).

## Repo layout (single Go module)

```text
cmd/server/  cmd/agent/
internal/api/       server handlers, auth middleware
internal/store/     git-backed file store (read tree, write+commit)
internal/syncer/    agent 3-way diff, slug translation, apply
internal/manifest/  shared types, sha256
build/server.Dockerfile  build/agent.Dockerfile
deploy/server/compose.yaml  deploy/client/compose.yaml
global/  projects/  docs/   (content unchanged)
```

## Security / hardening — accepted for now, open items

- Plain HTTP + static bearer token on LAN. TLS / reverse proxy, token
  rotation: later hardening step.
- Off-LAN machines: reach the server via VPN (e.g. tailscale) — not in
  scope.

## Rollout steps

1. E2E verified locally against fixture dirs (never real `~/.claude`).
2. Revert phase 0 symlinks to real files on this machine.
3. User deploys `deploy/server/compose.yaml` on the home box, sets
   token + GitHub deploy key.
4. Each machine runs `deploy/client/compose.yaml` with its env.
