# Phase 4 — usage/error mining to draft new rules (future, context only)

Not planned yet. This doc exists so a future session can start Phase 4
without re-deriving the following facts.

## Goal (as stated by user)

"Frequently used function and error should be logged and use this
information to create function and rules."

## Why this is plausible — a version of this pattern already exists manually

`~/.claude/rules/forgejo.md` already contains a self-reinforcing rule:
"Missing a helper you need (or reused a raw call twice)? Propose adding a
new `forgejo-*` wrapper to `~/.zshrc.d/forgejo.zsh`... don't just keep
one-off curls." That's this exact idea, done manually, scoped to one
domain (Forgejo). Phase 4 is generalizing it: instead of relying on the
user (or Claude, in-session) to notice repetition and propose a helper,
mine it out of logs automatically and surface a draft.

## Candidate data sources (not yet inspected)

- `~/.claude/history.jsonl` (1.4M on this machine) — session history,
  format not yet read/parsed.
- `~/.claude/telemetry/` — exists, contents/format not yet inspected.
- `~/.claude/settings.local.json`'s `permissions.allow` — a 180-entry,
  session-accumulated allowlist of Bash/tool patterns actually run. This
  is itself a signal of "commands used often enough to get allowlisted" —
  possibly a cheaper first-pass signal than parsing history.jsonl.

First implementation step of this phase, before anything else: read what
these actually contain and in what format.

## What "frequently used function and error" should produce

- Repeated shell commands/one-off curls not yet wrapped in a helper ->
  draft a new zsh function proposal (same shape as the existing
  `forgejo-*` pattern) — ties into docs/02 (harness sharing), since a
  newly-drafted helper needs to go somewhere synced.
- Repeated errors/fixes -> draft a new rule/memory entry capturing the
  root cause and fix (per this machine's existing `verify_dont_guess` /
  `permanent_fixes` feedback memories — the draft should describe _why_,
  not just _what_, so it can be judged like any other feedback memory).

## Hard constraint — must not violate

Output is **always a draft proposal for user approval**, never an
auto-applied change. This matches every existing feedback memory on this
machine about guessed fixes, unsolicited destructive actions, and
sudo/bypass shortcuts — an automated system drafting rules from usage
patterns is exactly the kind of thing that must not self-apply.

## Privacy constraint — must not violate

`history.jsonl` and shell history can contain sensitive command
arguments (tokens, connection strings, IDs) even in "innocuous" looking
commands. Mining must run **local-only**: raw history/telemetry never
leaves the machine it was generated on, never gets synced via Phase 1's
mechanism. Only the final aggregated/human-approved pattern (e.g. "user
runs X-shaped curl often, proposes wrapper Y") is eligible to sync.

## Open questions to resolve before drafting a real plan

- Exact schema of `history.jsonl` and `telemetry/` — unknown, read first.
- Threshold for "frequent enough to propose" — needs real data before
  picking a number, same caveat as docs/03's cadence question.
- Does this run per-machine (each machine mines its own local logs and
  proposes locally) or does it need aggregated cross-machine usage data
  routed through Phase 3's condense job? Per-machine is simpler and
  respects the privacy constraint above by default; cross-machine
  aggregation would need an explicit, deliberate design for what crosses
  the sync boundary.
