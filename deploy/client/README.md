# memory-agent deploy

Per-machine agent: syncs `~/.claude` memory with the hub server on a
ticker (up-sync local changes, down-sync canonical state).

## Quick start

```sh
cp .env.example .env   # edit values
docker compose pull && docker compose up -d
```

The image is published to GHCR (`ghcr.io/orfeo42/claude-memory-sync-agent`)
by the `publish` workflow on push to `main` — no repo checkout or build
needed, only `compose.yaml` + `.env`.

## Environment variables

| Variable             | Required         | Example                 | Meaning                                                                                |
| -------------------- | ---------------- | ----------------------- | -------------------------------------------------------------------------------------- |
| `MEMORY_SERVER_URL`  | yes              | `http://raspberry:8080` | Hub server base URL                                                                    |
| `MEMORY_TOKEN`       | yes              | —                       | Bearer token, must match the server's `MEMORY_TOKEN`                                   |
| `MEMORY_CLIENT_ID`   | yes              | `desktop`               | Unique name for this machine; server keeps a per-client snapshot under `clients/<id>/` |
| `MEMORY_SLUG_PREFIX` | yes              | `-home-orfeo42`         | Slug of this machine's home directory (see below)                                      |
| `MEMORY_INTERVAL`    | no (`15m`)       | `15m`                   | Sync cycle interval                                                                    |
| `CLAUDE_DIR`         | no (`~/.claude`) | `~/.claude`             | Local Claude directory to sync                                                         |

## Choosing `MEMORY_SLUG_PREFIX`

Project memory dirs under `~/.claude/projects/` are named after the
slugified absolute project path (e.g.
`-home-orfeo42-sviluppo-personal-claude-memory-sync`). The prefix is the
slug of `$HOME` itself:

```.env
MEMORY_SLUG_PREFIX=-home-<username-on-this-machine>
```

Every project dir starting with the prefix is synced; the prefix is
stripped so the canonical key is machine-independent — the same project
at the same path relative to home merges across machines even when
usernames differ. Project dirs outside home are skipped with a warning.

## Token from GNOME keyring (no plaintext .env)

```sh
MEMORY_TOKEN=$(secret-tool lookup service MEMORY_TOKEN user "$USER") docker compose up -d
```

Note: every compose command that touches this file needs the variables
resolved, including `down`.
