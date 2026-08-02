# Phase 9 — Windmill pipeline

## Decision record (2026-08-02)

The shell synthesizer (`build/synthesizer/intake.sh` + `synth.sh`,
compose services `memory-intake` / `memory-synthesizer`, docs/08) is
functionally complete but has three properties the user wants to change:
prompt text is baked into shell heredocs (editing a judgment prompt means
a rebuild+redeploy), there is no approval gate for anything beyond the
condense pass's post-hoc-audit model, and adding a new pass (staleness
judgment, cross-project distillation, hot-cache regeneration, harness
mining) means writing more `sh` against `lib.sh`'s helpers.

**Decision: run the pipeline on self-hosted Windmill CE** (a Raspberry Pi
compose service, `deploy/server/compose.yaml`: `windmill-db` +
`windmill-server` + `windmill-worker`), replacing the shell loop-driven
services with Windmill flows, over two alternatives considered and
rejected:

- **Forgejo Actions** (already available on `git.ionstream.ai`, see
  `~/.claude/rules/forgejo.md` for the org's existing Forgejo usage) —
  rejected: CI-runner semantics (ephemeral job containers, YAML workflow
  syntax built for build/test pipelines, no built-in human-approval
  primitive, no persistent variable store editable outside the workflow
  file) are a worse fit than a workflow engine's suspend/resume and
  variable store for a scheduled, occasionally-pausing-for-approval
  pipeline. Would also add a hard dependency on the org's shared Forgejo
  instance for a personal-project background job.
- **Custom Go scheduler** (a `cmd/synth-scheduler` in this repo, ticker +
  step functions, mirroring `cmd/agent`'s shape) — rejected: reimplements
  what Windmill already provides (schedule UI, run history, suspend/
  resume approval steps, a variable store editable without a redeploy)
  for no benefit specific to this project; the actual LLM-judgment logic
  moves into `memoryctl` regardless (see below), so a custom scheduler
  would mostly be reimplementing Windmill's own step-runner.

Windmill wins on the properties that motivated the change: **prompt
variables are UI-editable** (`deploy/windmill/prompts/*.md` are the
canonical defaults, pasted into workspace variables — editing a variable
changes the next scheduled run, no rebuild), **approval steps are a
first-class flow primitive** (`suspend`/resume, used for every
`global/**` proposal — distillation and task-mining outputs), and
**subscription-token-only auth carries over unchanged** (every flow step
starts with the same `ANTHROPIC_API_KEY` refusal guard the shell scripts
had, `CLAUDE_CODE_OAUTH_TOKEN` supplied as worker container env).

## What moved where

- **LLM-judgment logic**: the shell prompt-building + `parse_llm_ops` +
  `validate_op_filename` + `scan_op_secrets` + `rebuild_memory_index`
  helpers in `lib.sh` become subcommands of a new `memoryctl` binary
  (`cmd/memoryctl`, implemented alongside this doc — see its CLI contract
  in `deploy/windmill/setup.md`/the flow YAMLs). The worker image
  (`build/windmill-worker.Dockerfile`) is the official Windmill worker
  image plus `git`, the `claude` CLI, and the built `memoryctl` binary
  copied in.
- **Orchestration**: `intake.sh`'s client loop / project loop and
  `synth.sh`'s project loop become Windmill flows
  (`deploy/windmill/flows/*.flow.yaml`) — thin bash steps chaining
  `memoryctl` subcommands and `claude -p`, with control flow (`for`
  loops, retries, the approval branch) expressed as Windmill flow
  modules instead of POSIX shell.
- **Prompts**: the four inline heredoc prompts in `intake.sh`/`synth.sh`
  become the canonical default bodies of `deploy/windmill/prompts/
  intake-judge.md` and `synth-condense.md`, pasted into Windmill
  variables at setup time (`deploy/windmill/setup.md` §4). Three new
  prompts add passes the shell pipeline never had:
  `synth-staleness.md` (deletion judgment, gated behind the
  `staleness_enabled` variable — off by default, since `synth-condense.md`
  alone already matches the shell pipeline's non-destructive-by-default
  posture for entries condense doesn't judge as duplicates),
  `distill-scan.md`/`distill-write.md` (cross-project clustering +
  unification, two-stage), and `hotcache-digest.md` (per-project digest
  writer, feeding the still-unbuilt hot-cache tier from docs/05).
- **Harness mining**: `taskmine-draft.md` + `taskmine.flow.yaml` are new
  — nothing in the shell pipeline looked for repeated manual procedures
  across the corpus worth turning into a skill/script. Always
  proposal-only, weekly.

## Not removed (yet)

`memory-intake` / `memory-synthesizer` compose services and
`build/synthesizer/*.sh` stay in the repo and keep running. Windmill is
additive until proven, not a same-day cutover — see the parity-gate plan
below. A future doc will record the actual removal once that gate
passes; this doc does not delete the shell services.

## Components

| Component | What it is | Notes |
| --- | --- | --- |
| `windmill-db` | Postgres 16, Windmill's own state store | new compose service |
| `windmill-server` | Windmill API + UI, port 8081 | new compose service |
| `windmill-worker` | Windmill worker + `git` + `claude` CLI + `memoryctl` | `build/windmill-worker.Dockerfile`, mounts the shared `memory-data` volume at `/data` (same volume the shell services and `memory-server` use) |
| `intake.flow` | scan clients, mechanical config merge, per-project intake judgment, push, marker advance | schedule 45m, concurrency 1 |
| `daily-synth.flow` | per-project condense (+ optional staleness), cross-project distill (scan + write, proposal-gated), hot-cache digest, push | schedule daily, concurrency 1 |
| `taskmine.flow` | whole-corpus harness-mining, proposal-gated | schedule weekly, concurrency 1 |
| `u/admin/intake_judge`, `synth_condense`, `synth_staleness`, `hotcache_digest`, `distill_scan`, `distill_write`, `taskmine_draft` | prompt-text variables | one per `deploy/windmill/prompts/*.md`, UI-editable |
| `u/admin/synth_model`, `staleness_enabled`, `hotcache_max_bytes` | parameter variables | see `deploy/windmill/setup.md` §5 for defaults |
| `u/admin/claude_code_oauth_token` | secret variable | belt-and-suspenders alongside the worker's own container env — see setup.md §3 |

## Constraints carried over from docs/08 (still binding)

- **The canonical repo's whitelist is a fixed set of top-level paths**
  (`global/CLAUDE.md`, `global/rules/*.md`, `global/skills/**`,
  `global/agents/**`, `projects/*/memory/**` — `internal/githook`'s
  push-side whitelist, mirrored by the agent's local path mapping in
  `internal/syncer/localpath.go` on the down-sync side). Anything a
  Windmill flow commits to `canonical.git` **must** land inside one of
  those recognized paths, or it bricks down-sync: an agent's manifest
  diff has no local-path mapping for an unrecognized top-level path, so
  it can't apply what it can't map. This is why every flow's proposal
  staging (`/tmp/proposals` on the worker container) happens **off any
  git branch entirely** — proposals are plain files on local disk until
  `memoryctl proposal-apply` writes them into an already-recognized path
  (`global/rules/*.md`, `global/CLAUDE.md`, `global/skills/**`) and only
  then are they committed to `main`. No flow ever commits to a branch
  other than `main`, and none ever invents a new top-level directory.
- **Hot cache is a header block inside `MEMORY.md`, not a separate
  file.** `hotcache-digest.md` requires the model's output to start with
  the literal heading `## Hot cache`; `memoryctl hotcache-write` writes
  that block into the existing header region of `MEMORY.md` (the part
  `lib.sh`'s `rebuild_memory_index` already preserves verbatim — the
  lines before the first `- [` index entry). `daily-synth.flow`'s
  hotcache step therefore modifies `MEMORY.md` in place rather than
  creating a new file per project.
- **Marker semantics** (`refs/intake/last-processed` per client repo,
  advanced only after the client's whole diffed range is processed)
  carry over unchanged in meaning, but `intake.flow`'s failure semantics
  are coarser than the shell version's per-client `failed_clients`
  tracking: the flow's per-project loop uses `skip_failures: false`, so
  ANY project's judgment failure stops the run before markers advance at
  all — every client in that run retries its full range next time, not
  just the clients that touched the failed project. Documented as an
  accepted v1 simplification in `deploy/windmill/setup.md` §10.

## Cutover parity-gate plan

1. Deploy the Windmill stack alongside the still-running shell services.
   Do not disable `memory-intake`/`memory-synthesizer` yet.
2. Run each Windmill flow manually (not on its schedule) against the
   real `canonical.git`, one pass at a time, with the shell services'
   own schedule left running. Because both write through
   `memoryctl commit`/`push` (Windmill side) and `commit_with_retry`
   (shell side) against the same `/data/state/canonical.lock`, writes
   serialize safely even if a manual Windmill run overlaps a scheduled
   shell run — but the two are independent judgment passes over the same
   corpus, so expect to see near-duplicate commits during this phase,
   not corruption.
3. Compare: for several consecutive days, diff what `intake.flow`/
   `daily-synth.flow` would have produced against what the shell
   pipeline actually committed for the same client pushes / same nightly
   corpus state. Parity bar: Windmill's condense/intake decisions are at
   least as good (no regressions a human reviewer flags on the commit
   log) — exact byte-for-byte match is not the bar, LLM output is not
   deterministic between the two prompt-delivery paths.
4. Once parity holds, enable the Windmill schedules (§7 of setup.md) and
   stop the shell services' schedules first — e.g. scale
   `memory-intake`/`memory-synthesizer` to zero replicas, or comment out
   their compose services — **before** deleting `build/synthesizer/*`
   from the repo. Keep the shell scripts in git history/on disk for one
   more review cycle as a rollback path.
5. Only after a further stable period, remove `build/synthesizer/*`,
   the `memory-intake`/`memory-synthesizer` compose services, and this
   section's "not removed yet" framing in a follow-up doc.

## Open / deferred

- Selective (per-proposal) approval — v1's suspend/resume approves or
  discards an entire batch of staged proposals, no per-file granularity.
- The Obsidian-adjacent knowledge-structure UI from docs/05 is still not
  built; Windmill's own run history is the only current visibility into
  what a pass changed, beyond `git log` on `canonical.git`.
