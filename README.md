# claude-memory-sync

Centralize Claude Code memory (`CLAUDE.md`, `rules/`, per-project
`memory/`) across an open-ended number of personal machines. Hub model:
a Go API server container on an always-on home box is the single sync
point; each machine runs a Docker agent container (only per-machine
dependency: Docker).

**This repo is CODE ONLY** (server, agent, deploy, docs). Memory
content never lives here. Content locations: live files in `~/.claude`
per machine; the hub's git-backed storage volume; and a separate
private GitHub repo (`claude-memory`, content backup) the server
pushes to daily — backup only, never the sync path.

Multi-step effort, not a one-shot build. Status per phase below.

## Phases

| Phase | Doc                                                                              | Status                                 |
| ----- | -------------------------------------------------------------------------------- | -------------------------------------- |
| 0     | [docs/00-repo-and-scaffold.md](docs/00-repo-and-scaffold.md)                     | Done (2026-07-19)                      |
| 1     | [docs/01-hub-sync.md](docs/01-hub-sync.md)                                       | Implemented + e2e green — rollout pending |
| 2     | [docs/02-plugin-harness-sharing.md](docs/02-plugin-harness-sharing.md)           | Future — context captured, not planned |
| 3     | [docs/03-condense-dedup.md](docs/03-condense-dedup.md)                           | Future — context captured, not planned |
| 4     | [docs/04-usage-mining-rules.md](docs/04-usage-mining-rules.md)                   | Future — context captured, not planned |
| 5     | [docs/05-hot-cache-rollup.md](docs/05-hot-cache-rollup.md)                       | Future — context captured, not planned |
| 6     | [docs/06-transcript-knowledge-mining.md](docs/06-transcript-knowledge-mining.md) | Future — context captured, not planned |

## Ground rules (apply to every phase)

- Hub-based, containerized: sync goes through the home-box API server;
  the only per-machine dependency is Docker. GitHub is never the sync
  path — backup pushes happen server-side only. (Replaced the original
  "local-first P2P" rule, 2026-07-19 — see docs/01.)
- Whitelist, not blacklist: only explicitly-listed paths ever leave
  `~/.claude`. A future Claude Code update adding new sensitive files under
  `~/.claude` must not silently get swept into sync.
- Never sync: `sessions/`, `ide/`, `daemon/`, `.credentials.json`, `cache/`,
  `paste-cache/`, `telemetry/`, `history.jsonl`, `shell-snapshots/`,
  `jobs/`, `tasks/`, `file-history/`.
- Condense/dedup (phases 3/5) may auto-apply, with git as the safety
  net: one commit per logical change, reviewable/revertable after the
  fact (post-hoc audit — decided 2026-07-19, see docs/05).
  Rule-generation (phase 4) still requires explicit human pre-approval —
  never auto-applied.
- Machine count/list is intentionally open-ended (1..n) — nothing here
  should hardcode a fixed set of hosts.
