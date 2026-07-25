# Phase 6 — project knowledge mining from session transcripts (future, context only)

Not planned yet. This doc exists so a future session can start Phase 6
without re-deriving the following facts.

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
