# Phase 3 — condense/dedup (future, context only)

Not planned yet. This doc exists so a future session can start Phase 3
without re-deriving the following facts.

## Goal (as stated by user)

"The server should cleanup the memory condensing the information and
removing duplications." Once multiple machines are pushing per-project
memory (Phase 1) and multiple machines may independently write similar
feedback/project entries, memory content needs periodic consolidation.

## Why this is needed, concretely

- Phase 1's hub sync (docs/01) never merges: each machine mirrors its
  memory into its own server-side namespace (`clients/<client-id>/`).
  Cross-machine reconciliation IS this phase — condensation reads all
  client namespaces and writes the merged result to `canonical/`,
  which is the only tree agents pull. Until this phase runs,
  per-project memory accumulates per-client and does not flow back.
- Even without conflicts, two machines can independently write
  semantically-duplicate memory entries (e.g. both notice "verify before
  claiming a fix works" and each write their own `feedback_*.md`) — the
  existing memory system's own instructions already warn against this
  ("Do not write duplicate memories. First check if there is an existing
  memory you can update before writing a new one") but that check is
  per-session/per-machine, not cross-machine.

## Candidate approach (not committed to)

- A scheduled job (candidate mechanism: the `schedule` skill / CronCreate,
  which already exists in this Claude Code install) that periodically
  reads all `MEMORY.md` + `memory/*.md` across every project dir in the
  synced repo (`projects/<canonical-key>/memory/`) plus any pending
  `*.sync-conflict-*` files.
- Needs a definition of "duplicate" beyond exact-text match — likely an
  LLM-judgment pass (semantic similarity), not a hash/diff.
- ~~Must never auto-apply merges.~~ **Amended 2026-07-19 (see docs/05):
  condense/dedup may auto-apply under the post-hoc audit model** — one
  git commit per logical change, reviewable/revertable after the fact.
  Phase 3 still owns the _semantics_ (what counts as duplicate, merge
  vs split); docs/05 runs them on a daily schedule.

## Open questions to resolve before drafting a real plan

- Where does the review happen — a generated diff the user reads and
  approves via a normal git PR-style flow in this repo, or an interactive
  Claude Code session that walks through proposed merges one at a time?
- Cadence: how often should this run? Too frequent = noisy review burden;
  too rare = duplicates pile up. No data yet on how fast duplicates
  actually accumulate — worth instrumenting before picking a number.
- Does condensing ever need to _split_ an over-broad memory entry (the
  inverse of merging), or is merge-only sufficient for the observed
  failure mode?
- Relationship to docs/04 (usage mining): both phases produce
  human-review-gated changes to memory/rules — should they share one
  review mechanism instead of two separate ones?
