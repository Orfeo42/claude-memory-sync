# Phase 0 — repo + scaffold

## Goal

Stand up this repo as the single staging point for everything that gets
synced. Whitelist design: only known-safe paths ever get copied out of
`~/.claude` — never a blacklist over the whole directory.

## Current state (surveyed on orfeo-pc, 2026-07-12)

- `~/.claude` is not a git repo, no `.gitignore`.
- `~/.claude/CLAUDE.md` — 89 lines / 7.1K, global (no path-dependency).
- `~/.claude/rules/*.md` — 9 files (`angular.md`, `forgejo.md`, `go.md`,
  `justfile.md`, `openstack.md`, `projects.md`, `python.md`, `react.md`,
  `ts.md`), global.
- `~/.claude/projects/<slug>/memory/` — one dir per working-directory path
  ever used, `<slug>` = absolute cwd path with `/` replaced by `-` (e.g.
  `-home-orfeo42-sviluppo-ionstream-billing-backend`). Each holds
  `MEMORY.md` (index) + `feedback_*.md`/`project_*.md`/etc files.
- Sensitive dirs never to touch: `sessions/`, `ide/`, `daemon/`,
  `.credentials.json`, `cache/`, `paste-cache/`, `telemetry/`,
  `history.jsonl`, `shell-snapshots/`, `jobs/`, `tasks/`, `file-history/`,
  `backups/` (unverified — check before including anything from it).

## Target structure

> **Superseded 2026-07-19 (repo split):** `global/` and `projects/`
> were removed from this repo — it is code-only now. Memory content
> lives in the hub storage volume + a separate `claude-memory` backup
> repo (see docs/01). Structure below kept for history.

```text
claude-memory-sync/
├── README.md
├── docs/                      (this planning doc set)
├── .gitignore                 (.stfolder, *.sync-conflict-*)
├── global/
│   ├── CLAUDE.md              <- moved from ~/.claude/CLAUDE.md
│   └── rules/*.md             <- moved from ~/.claude/rules/
└── projects/
    └── <canonical-key>/
        └── memory/*.md        <- see docs/01 for the path-translation scheme
```

## Steps (executed 2026-07-19)

1. `git init` — done (first commit `682c345` by user).
2. `.gitignore`: `.stfolder`, `*.sync-conflict-*`, `.obsidian/` (last
   one per docs/05's Obsidian-viewer decision).
3. GitHub repo created by user: `github.com:Orfeo42/claude-memory-sync`,
   private. Remote is named **`origin`** (not `backup` as originally
   drafted) — GitHub's role is still backup-only per ground rules, only
   the remote name differs.
4. Moved `~/.claude/CLAUDE.md` -> `global/CLAUDE.md`, `~/.claude/rules/`
   -> `global/rules/`; originals replaced with absolute symlinks into
   this repo. Verified: content resolves through both links.
   **Superseded 2026-07-19**: the hub redesign (docs/01) syncs global
   files as real files via the agent container. Symlinks REVERTED same
   day (user: repo deletion must never break live config) — `~/.claude`
   holds real copies again; repo `global/` is the sync staging copy.
5. First commit + push done by user before step 4; the move/symlink
   changes are committed by the user as usual.

## Findings from execution (resolved open items)

- `~/.claude/projects/<slug>/` is **NOT** memory-only: every slug dir
  also holds session transcripts (`<uuid>.jsonl`) and session subdirs
  (`<uuid>/`). Phase 1's push script MUST copy only the `memory/`
  subdir — confirmed as a hard requirement, not a precaution.
- Repo lives at `~/sviluppo/personal/claude-memory-sync` (docs/01's
  examples say `~/sviluppo/claude-memory-sync` — the `personal/` segment
  is the real path; symlink targets above use it).
