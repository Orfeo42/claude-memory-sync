# Windmill pipeline — setup runbook

This directory holds the Windmill-hosted replacement for the shell
synthesizer pipeline (`build/synthesizer/intake.sh` / `synth.sh`): prompt
text as importable defaults (`prompts/*.md`), flow templates
(`flows/*.flow.yaml`), and this runbook. The compose services
(`windmill-db`, `windmill-server`, `windmill-worker`) already live in
`deploy/server/compose.yaml`; the worker image
(`build/windmill-worker.Dockerfile`) bundles `memoryctl`, `git`, and the
`claude` CLI on top of the official Windmill worker image.

## Flow YAML caveat — read this first

The three `flows/*.flow.yaml` files are **templates**, not verified
Windmill OpenFlow exports. They were written against the general shape of
Windmill's flow format (module list, `rawscript` steps, `forloopflow`,
`branchone`, `suspend`) without a running instance to validate against.
Concretely uncertain, and marked `TODO verify` inline in the YAML:

- The exact field names for per-flow concurrency limits
  (`concurrent_limit` / `concurrency_time_window_s` at `value` level).
- The `$var:f/memory/<name>` static-value syntax for resolving a Windmill
  variable into a flow input default — confirm this resolves at run time
  on the installed version; if not, replace with an explicit step that
  calls the variable-get API/CLI.
- `branchone` module shape (`branches[].expr` + `branches[].modules`,
  `default: []`).
- `suspend` module config (`required_events`, `timeout`) and how the
  resume payload is exposed to later steps.

**On import: open each flow in the Windmill flow editor, let it flag
whatever it doesn't recognize, and fix step-by-step rather than trusting
a clean YAML parse as proof of correctness.** `python3 -c "import yaml,
glob; [yaml.safe_load(open(f)) for f in
glob.glob('deploy/windmill/flows/*.yaml')]"` only proves the files are
syntactically valid YAML, not valid OpenFlow.

## 1. First boot

1. Bring up the DB + server: `docker compose up -d windmill-db
windmill-server` (from `deploy/server/`). Requires
   `WINDMILL_DB_PASSWORD` set in `.env`.
2. Open `http://<host>:8081`. First visit prompts you to create the
   **superadmin** account (email/password, local to this instance — not
   tied to any external auth).
3. Create a workspace named `memory` (Windmill ships a default `demo`
   workspace; do not use it — keep this pipeline isolated). All variables,
   flows, and schedules below live inside the `memory` workspace.
4. Only now bring up the worker: `docker compose up -d windmill-worker`.
   It needs `CLAUDE_CODE_OAUTH_TOKEN` set (see step 3 below) to do
   anything useful, but will start regardless.

## 2. Install the `wmill` CLI (optional but recommended)

The Windmill CLI (`wmill`) can push variables/flows/schedules from files
instead of the UI, which is worth it once you're iterating on the flow
YAMLs. Install per Windmill's own docs (`npm install -g windmill-cli` at
time of writing — verify against the installed server's version, the CLI
and server versions should match). Log in with `wmill workspace add` and
the superadmin token from step 1. Everything below is written assuming
UI usage; substitute the equivalent `wmill` commands if you have the CLI
set up.

## 3. Secrets

In the `memory` workspace, go to **Variables** → **New variable**:

- Name: `f/memory/claude_code_oauth_token`
- Value: the output of `claude setup-token` (subscription OAuth, no API
  credits — see `deploy/server/README.md` for the existing synthesizer's
  version of this same one-time setup).
- Mark it **Secret** (encrypts at rest, hides value in the UI/logs).

Note: `deploy/server/compose.yaml`'s `windmill-worker` service already
receives `CLAUDE_CODE_OAUTH_TOKEN` as a container environment variable
(required, `${CLAUDE_CODE_OAUTH_TOKEN:?}`), which is what the `claude`
CLI actually reads inside every flow step — the container env takes
effect regardless of whether a Windmill variable of the same name
exists. Creating the workspace secret above is still worth doing for
visibility/rotation from the Windmill UI, but no flow step needs to
fetch it explicitly; it is already in the step's process environment.

**Never set `ANTHROPIC_API_KEY`** anywhere in this chain (container env,
workspace variable, or flow input) — every bash step in every flow
starts with a guard that refuses to run if it is set, specifically to
avoid silently billing API credits instead of the subscription.

## 4. Prompt variables

Create one **non-secret** string variable per file below, pasting the
file's full content as the variable's value. These are the defaults the
flows reference; editing a variable's value in the UI changes the next
scheduled run immediately, no image rebuild or redeploy needed.

| Variable path               | Source file                   | Used by            |
| --------------------------- | ----------------------------- | ------------------ |
| `f/memory/intake_judge`     | `prompts/intake-judge.md`     | `intake.flow`      |
| `f/memory/synth_condense`   | `prompts/synth-condense.md`   | `daily-synth.flow` |
| `f/memory/synth_staleness`  | `prompts/synth-staleness.md`  | `daily-synth.flow` |
| `f/memory/hotcache_digest`  | `prompts/hotcache-digest.md`  | `daily-synth.flow` |
| `f/memory/distill_scan`     | `prompts/distill-scan.md`     | `daily-synth.flow` |
| `f/memory/distill_write`    | `prompts/distill-write.md`    | `daily-synth.flow` |
| `f/memory/taskmine_draft`   | `prompts/taskmine-draft.md`   | `taskmine.flow`    |
| `f/memory/taskmine_cleanup` | `prompts/taskmine-cleanup.md` | `taskmine.flow`    |

## 5. Parameter variables

Also as non-secret string/boolean/integer variables (or leave unset and
rely on the flow input schema defaults documented in each
`flows/*.flow.yaml` — either works, an explicit variable makes the
current value visible without opening a flow run):

| Variable path                 | Type    | Default  | Meaning                                               |
| ----------------------------- | ------- | -------- | ----------------------------------------------------- |
| `f/memory/synth_model`        | string  | `sonnet` | `claude -p --model` value, all flows.                 |
| `f/memory/staleness_enabled`  | boolean | `false`  | Appends `synth_staleness` to the condense prompt.     |
| `f/memory/hotcache_max_bytes` | integer | `4000`   | Cap passed to `memoryctl hotcache-write --max-bytes`. |

## 6. Import the flows

For each of `flows/intake.flow.yaml`, `flows/daily-synth.flow.yaml`,
`flows/taskmine.flow.yaml`, `flows/proposal-apply.flow.yaml`: **Flows**
→ **New flow** (or open the existing one) → **Import from YAML**, paste
the file, then work through whatever the editor flags per the caveat in
the section above. Live paths: `f/memory/intake`,
`f/memory/daily_synth`,
`f/memory/taskmine`,
`f/memory/proposal_apply` — the approval app starts the
last one by that exact path.

## 7. Schedules

Each flow gets its own schedule, attached from the flow's **Triggers** →
**Schedule** tab:

| Flow                  | Cron (6-field, seconds first) | Interval              |
| --------------------- | ----------------------------- | --------------------- |
| `intake.flow`         | `0 */45 * * * *`              | 45 minutes            |
| `daily-synth.flow`    | `0 0 3 * * *`                 | daily (03:00)         |
| `taskmine.flow`       | `0 0 4 * * 7`                 | weekly (Sunday 04:00) |
| `proposal-apply.flow` | `0 */10 * * * *`              | 10 minutes            |

Set `no_flow_overlap: true` on every schedule. This instance's cron
parser takes a leading seconds field and rejects Sunday as `0` (use
`7`).

Pick times that don't overlap on a small Raspberry Pi worker — the
suggested offsets above stagger daily-synth and taskmine an hour apart
so a slow daily-synth run doesn't collide with taskmine.

## 8. Concurrency limit

Each flow template sets `concurrent_limit: 1` in its `value` block (see
the caveat section — verify this is the field the installed version
actually reads; if not, set the equivalent limit from the flow's
**Settings** → **Concurrency** tab in the UI). The intent is the same as
the shell pipeline's `flock` on `/data/state/canonical.lock`: never let
two runs of the same flow execute at once. Cross-flow concurrency
(intake running while daily-synth runs) is not restricted the same way —
`memoryctl ensure-work`/`commit`/`push`/`proposal apply-approved` take
the lock argument (`/data/state/canonical.lock`) precisely so
concurrent flows still serialize their actual canonical writes even if
Windmill lets both flows be "running" simultaneously. `ensure-work`
never resets away unpushed commits another flow left in the shared work
dir (see `.docs/11-pipeline-state-db.md`). This relies on the `default`
worker group having exactly one worker: flow steps then never run in
parallel, so no step can be mid-write while another syncs the work dir.

## 9. Approval steps

`daily-synth.flow` (distillation stage) and `taskmine.flow` post
proposals to `pipeline-api` (`memoryctl ops-apply --source-flow
--source-run`) instead of applying them — anything destined for
`global/rules/**`, `global/CLAUDE.md`, or `global/skills/**` is
proposal-only, never auto-applied (same rule as the original
condense-pass design in `.docs/03-condense-dedup.md`). Both flows then
finish normally: no run ever suspends, and canceling or deleting runs
can no longer lose a pending proposal. Design and status machine:
`.docs/11-pipeline-state-db.md`.

Resolve proposals in the approval inbox app (`app/APP.md`), one by one:

- **Approve** — moves it to `approved` and starts
  `f/memory/proposal_apply`, which claims it, writes and commits it under
  the canonical lock (`proposal: apply <kind> <name>`), pushes, and only
  then marks it `applied` with the commit sha. A push failure marks it
  `failed`; **Retry** in the app re-queues it.
- **Reject** — moves it to `rejected` and records `(kind, name)` as a
  rejection; taskmine's draft prompt receives every rejection as a
  do-not-re-propose list, plus the names of proposals still pending or
  approved and the inventory of skills already in `global/skills/`.
- Pending proposals older than 14 days expire automatically
  (`proposal-apply`'s first step).

After an applied **skill**, `proposal-apply`'s cleanup step runs the
former taskmine post-promotion cleanup: the entry refs in the skill's
Evidence section (`<project-key>/<file>.md`) are extracted mechanically,
and per project a claude judge decides for each cited entry whether it
is now fully covered by the skill (delete), partially covered (trim,
with a pointer to the skill), or independently valuable (keep). Applied
via `ops-apply --allow-delete`, one commit per project
(`taskmine: cleanup <key> after <name>`).

### One-time migration from the suspend-based approvals

Inside `windmill-worker-ui-backend` (it holds the admin token):

```sh
for d in /data/state/proposals/*/; do
  memoryctl proposal import --dir "$d" --source-flow legacy-import
done
memoryctl proposal import-rejections --file /data/state/taskmine-rejected --actor legacy-import
```

After one clean `proposal-apply` cycle, delete `/data/state/proposals/`
and `/data/state/taskmine-rejected`.

## 10. Known v1 simplifications (carried over from the implementation brief)

- **intake.flow**: the per-project loop uses `skip_failures: false`, so
  any single project's judge-step failure stops the whole run before the
  push/marker-advance steps run. Every client's `refs/intake/last-processed`
  marker stays unadvanced and the full range is retried next run. This is
  coarser than `intake.sh`'s original per-client `failed_clients`
  tracking (which let unaffected clients' markers advance even when one
  project failed) — accepted for v1.
- **daily-synth.flow distill stage**: `distill-write.md` asks the model
  to pick "the single most relevant project" for the unified
  `===FILE:===` entry, but `memoryctl ops-apply` takes one
  `--memory-dir` per invocation. The flow resolves the target
  mechanically as the **first project listed** in the cluster's
  `entries[]` array (from `distill-scan.md`'s stage-1 output) rather
  than parsing the model's choice back out of its own output. If the
  model's free-form judgment and the mechanical first-entry pick
  diverge, the entry still lands under the first-entry project.
- **daily-synth.flow hotcache stage**: the commit after
  `memoryctl hotcache-write` uses pathspec `projects/<key>` (the whole
  project directory) rather than a more precise path, because the CLI
  contract does not pin down exactly where `hotcache-write` places its
  output file relative to `--memory-dir`.

## 11. Verify before relying on it

Run each flow manually once from the UI (**Run** button, fill in the
input form with its schema defaults) with a small/test canonical repo
before trusting the schedules. Confirm: the `ANTHROPIC_API_KEY` guard
actually blocks a step when the var is set (test deliberately once);
`memoryctl push` lands commits visible via `git log` on
`/data/repos/canonical.git`; one real proposal goes pending → approved
→ applied through the app, with its commit sha visible in the app's
event history and in `git log` on canonical.
