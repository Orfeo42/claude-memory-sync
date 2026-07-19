# Phase 5 — daily synthesis, hot cache, knowledge UI (future, context only)

Not planned yet. This doc exists so a future session can start Phase 5
without re-deriving the following facts.

## Origin

2026-07-19: evaluated adopting
[AgriciDaniel/claude-obsidian](https://github.com/AgriciDaniel/claude-obsidian)
(self-organizing Obsidian vault, 15 skills). Decision: **skip adoption**
— it solves knowledge management on one machine, not cross-machine
memory sync; no evidence of Claude Code performance gains (its only
benchmark measures its own retrieval accuracy, no published
methodology); 15-skill maintenance surface is the exact pattern the
community reports abandoning within weeks. Two of its architectural
ideas are kept (hot-cache tier, fold/rollup), extended here with two
user requirements: a daily automatic synthesis job and a read-only
knowledge-structure UI.

## Decision — post-hoc audit replaces pre-approval (2026-07-19, user)

The daily synthesis job runs **fully automatic, with git as the safety
net**: it may merge/rewrite source `memory/*.md` files directly, but
every logical change lands as a **separate git commit** the user can
review and revert after the fact.

Scope of this amendment: **condense/dedup only (phases 3/5)**. Phase 4
rule-_generation_ stays pre-approval-gated — automatically drafting new
behavior rules is a different risk class than merging duplicate memory
entries, and keeps its "always a draft proposal" hard constraint.

README ground rules carry the amended wording; docs/03 points here.

## Daily synthesis pipeline

Once-a-day job (systemd user timer, consistent with the Phase 1
decision: timers, not cron) that keeps the memory corpus clean and
feeds the hot cache:

1. **Collect** — read every client namespace on the server
   (`clients/<client-id>/projects/<key>/memory/*.md` + `MEMORY.md`
   indexes) plus the current `canonical/` tree (see docs/01).
2. **Synthesize** — LLM pass that removes duplication (semantic, not
   hash — same definition problem as docs/03) and ambiguity
   (contradictory or vague entries get rewritten to one clear form).
3. **Apply** — write the merged/cleaned result to `canonical/` (client
   namespaces stay untouched — they are raw history). One git commit
   per logical change (merge of two entries = one commit) so each is
   independently revertable.
4. **Regenerate cache** — rebuild the hot cache (below) from the
   cleaned source.

This subsumes docs/03's condense step: Phase 3 defines the _semantics_
(what counts as duplicate, merge vs split), Phase 5 runs them on a
recurring schedule under the post-hoc audit model.

## Hot-cache tier

claude-obsidian keeps a three-tier structure: a ~500-word "hot cache"
of recent/most-relevant context always loaded, a master index, and full
pages behind it.

Mapping to this project: the existing memory system already has two
tiers — `MEMORY.md` (index, loaded every session) and `memory/*.md`
(full entries, loaded on demand). The missing piece is a size-bounded
"currently hot" digest distinct from the index: not one line per
memory, but what matters _right now_ across the synced machines
(active projects, recent decisions, fresh feedback).

Shape: a generated `HOT.md` (per project dir or one global — open
question), rebuilt only by the daily synthesis job — never
hand-maintained, always derivable from the underlying entries, so a
sync conflict on it is resolved by regenerating, not merging.

## Knowledge-structure UI (Obsidian)

User requirement: a way to _see_ the knowledge structure, read-only.

Obsidian covers this natively, zero code: memory entries already link
each other with `[[wikilinks]]`, and an Obsidian vault is just a
markdown folder. Open the synced repo directory as a vault → graph
view, backlinks, local graph per note.

- **Read-only convention**: Obsidian is a viewer. Edits keep flowing
  through Claude Code sessions + the synthesis job; hand-edits in
  Obsidian would bypass the memory-format frontmatter conventions.
- **`.obsidian/` handling**: Obsidian writes its config dir into the
  vault root. Default: add `.obsidian/` to this repo's `.gitignore` and
  leave it out of any synced-path whitelist (per-machine viewer config,
  not memory). Syncing it deliberately is possible later if identical
  UI config across machines turns out to matter.

## Relationship to other phases

- Depends on Phase 1 (sync working) and Phase 3 (dedup semantics
  defined). Phase 3's open question about a shared review mechanism
  with Phase 4 narrows to Phase 4 only — Phase 5 no longer needs a
  pre-approval review mechanism, it needs a _revert_ path (git, already
  there via Phase 0/1 commits).

## Open questions to resolve before drafting a real plan

- Which machine runs the daily job? All machines running it would race
  (two synthesis passes merging the same entries concurrently — same
  locking concern as Phase 1's push/pull, but worse because it
  rewrites). Options: one designated machine, or a lock file synced
  via the repo. Needs a real design, not a default.
- Per-project `HOT.md` vs one global — depends on whether sessions are
  ever cross-project.
- How does the hot cache actually get _loaded_ into sessions?
  `MEMORY.md` is loaded by the harness; a sibling `HOT.md` is not.
  Candidate: synthesis job writes the digest into a marked section of
  `MEMORY.md` instead of a separate file. Decide when building.
- Is the hot cache needed at current scale at all? Measure real
  `MEMORY.md` sizes across projects before building the tier.
- Synthesis cadence is fixed daily by requirement, but the LLM pass
  cost/quality tradeoff (which model, how much context per run) is
  unmeasured.
