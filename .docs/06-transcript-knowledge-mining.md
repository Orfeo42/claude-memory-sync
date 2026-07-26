# Phase 6 — project knowledge mining from session transcripts

## Status (2026-07-26)

**Implemented as the `memory-mine` skill** (`~/.claude/skills/memory-mine/`,
syncs via phase 2a — code travels, transcripts and mining state do not):

- `bin/mine-slim` — jq compaction of a transcript: USER/ASSISTANT text
  only, tool results dropped, 3000 chars/message, 150KB/transcript cap.
- `bin/mine-project [--dry-run] <slug>` — incremental miner: skips
  sessions listed in `~/.local/state/memory-mine/<slug>.mined`
  (machine-local, NOT synced), skips tiny transcripts (<400 bytes
  slimmed), otherwise one `claude -p` call (model `$MINE_MODEL`,
  default sonnet) with existing MEMORY.md + file list for dedup,
  parses `===FILE:`-delimited output (`bin/mine-parse`), scrubs via
  secret-pattern regexes (reject, never edit), writes accepted entries
  to `memory/` and appends MEMORY.md index lines.
- `bin/mine-all [--dry-run]` — backfill driver over all project dirs.

Open questions resolved: transcript schema inspected (`type`
user/assistant, `.message.content` string or block array); session
subdirs (`<uuid>/subagents`, `tool-results`) carry implementation
detail only — v1 mines main transcripts only; scrubbing = prompt rule
+ mechanical regex pass (belt and braces); backfill runs per machine
on its own local transcripts (local-mining decision, 2026-07-26).

Pilot run (3d-printer project, 1 transcript): produced one high-quality
entry with wikilink + index line. Full backfill (353 transcripts,
151MB raw on this machine) pending user go — cost decision.
Recurring cadence still deferred to phase 5's daily timer.

## Goal (as stated by user, 2026-07-19)

"Collect and condense the project-specific knowledge/history." Source
scope decided with user: **session transcripts only** (not git history,
not repo docs), in two modes:

1. **One-time backfill** — mine all existing per-project session
   transcripts accumulated so far.
2. **Recurring pickup** — new sessions keep producing transcripts;
   extraction keeps running so knowledge doesn't silently accumulate
   unmined again.

## Source data (verified during Phase 0, 2026-07-19)

`~/.claude/projects/<slug>/` holds, per project:

- `<uuid>.jsonl` — one transcript per session (hundreds across ~16 slug
  dirs on this machine).
- `<uuid>/` — session subdirs.
- `memory/` — the only part that syncs (Phase 1 whitelist).

Transcripts are the raw history of every session: decisions made, bugs
root-caused, gotchas hit, approaches rejected. Today none of that
survives unless a session explicitly wrote a memory entry.

## What extraction produces

Per project: new/updated `memory/*.md` entries in the existing memory
format (frontmatter + `[[wikilinks]]`, `MEMORY.md` index line), holding
durable knowledge only — decisions + why, root causes + fixes,
constraints discovered, approaches rejected + why. Not a session log:
ephemeral back-and-forth stays out. Extraction output flows into the
normal pipeline: syncs via Phase 1, deduped/condensed by Phases 3/5.

## Hard constraints

- **Privacy (same as Phase 4):** transcripts contain sensitive command
  arguments, tokens, file contents. Mining runs **local-only**; raw
  transcripts never sync. Extracted entries must be scrubbed — no
  secrets, no credentials, no verbatim sensitive output — before they
  land in `memory/` (which does sync).
- **Audit model:** follows the Phase 5 post-hoc audit decision — writes
  land directly, one git commit per project per extraction run,
  reviewable/revertable. Distinction kept: this phase writes _memory
  entries_ (knowledge), never _rules_ (behavior) — drafting rules from
  history stays Phase 4, pre-approval-gated.

## Relationship to other phases

- Independent of Phases 1-2 (mining is local; useful even before sync
  works — output just sits in local `memory/` until Phase 1 syncs it).
- Feeds Phases 3/5: extraction will produce duplicates (same gotcha hit
  in many sessions) — dedup downstream, don't over-engineer dedup into
  the extractor.
- Recurring mode should ride Phase 5's daily timer (extract, then
  synthesize, then regenerate cache — natural order within one run)
  rather than adding a second scheduler.
- Phase 4 mines _usage patterns_ (commands, errors) for rule proposals;
  this phase mines _project knowledge_ for memory entries. Same raw
  sources, different output, different gate.

## Open questions to resolve before drafting a real plan

- Transcript schema: `.jsonl` line format not yet inspected — read
  before designing the extractor.
- Cost/batching: hundreds of transcripts, some huge. Which model runs
  extraction, how many transcripts per pass, incremental tracking of
  already-mined sessions (mtime? processed-list file?) — needs a real
  design; backfill is the expensive part, recurring pickup is cheap.
- Session subdirs (`<uuid>/`) — contents not yet inspected; determine
  whether they add signal or transcripts alone suffice.
- Scrubbing mechanism: LLM-judgment only, or also a mechanical
  secret-pattern pass (regexes for tokens/keys) as belt-and-braces?
- Does backfill process the current machine only, or must each machine
  backfill its own local transcripts once Phase 1 pairs them?
