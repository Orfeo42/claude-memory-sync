# Phase 10 — one-shot IaC bootstrap for the Windmill pipeline

Goal: a fresh server box goes from bare Docker to a fully working stack — containers, Windmill workspace `memory`, flows, variables, schedules, approval app, secrets — with one command, and the repo stays the single source of truth afterwards (no more paste-into-UI drift like the retired-shell-services drift found 2026-08-30).

## Key finding: there is no Terraform provider for Windmill

Verified 2026-08-30: the Terraform registry has no Windmill provider, official or community, and windmill-labs publishes none. Windmill's official IaC mechanism is the `wmill` CLI (`wmill sync push`/`pull`), which covers scripts, flows, apps, variables, resources, folders, schedules, triggers and workspace settings, computes diffs like a plan step, and skips secrets by default (`skipSecrets: true` in `wmill.yaml`). Wrapping `wmill` calls in Terraform `null_resource`/`local-exec` would add state-file liability without adding plan/diff value, because `wmill sync` already is the declarative diff engine. Decision: Terraform is dropped as the tool; the goal ("autosetup everything in one go") is met with `wmill sync` + the existing compose file, orchestrated by `just` recipes.

## Layers

| layer | what | tool | status |
| ----- | ---- | ---- | ------ |
| 0 | host: Docker, ssh, keyring secret | manual, once per box | as today |
| 1 | containers (server, windmill-db/server/worker-llm-steps/worker-ui-backend) | `deploy/server/compose.yaml` | done — already declarative |
| 2 | Windmill instance + workspace bootstrap (first admin, workspace `memory`, token) | `wmill` + admin API | to build |
| 3 | workspace content (flows, variables, schedules, app) | `wmill sync push` from a new `deploy/windmill/workspace/` tree | to build |
| 4 | secrets (`CLAUDE_CODE_OAUTH_TOKEN`, admin token) | keyring → `wmill variable` push at bootstrap time | to build |
| 5 | orchestration + drift gate | `just bootstrap`, CI `wmill sync pull --dry-run` diff | to build |

## Layer 1 — containers (no change, one hardening)

Compose stays as-is. One hardening folded in: pin the Windmill image instead of `windmill:main` (live server is CE v1.790.0) and pin the same version in `build/windmill-worker.Dockerfile`, so a fresh bootstrap reproduces the tested stack instead of whatever `main` is that day. Version bumps become explicit commits.

## Layer 2 — instance/workspace bootstrap

- First-run superadmin: Windmill CE creates the first admin interactively on first login. Verify whether CE honors the documented instance-bootstrap env/API for non-interactive first-admin creation; if not, this stays the single manual step (one browser login), everything after is scripted.
- Workspace: `wmill workspace add memory memory http://<host>:8081` with the admin token; idempotent (re-add is a no-op with same settings).
- Worker groups need no bootstrap: `default` and `ui-backend` register themselves from the compose env (`WORKER_GROUP`/`WORKER_TAGS`).

## Layer 3 — workspace content as a `wmill sync` tree

New directory `deploy/windmill/workspace/` in the `wmill sync` layout (`f/memory/...`), replacing the current mixed layout (`flows/*.flow.yaml` + `prompts/*.md` + APP.md paste instructions):

- **Flows** — the three existing flow YAMLs move into the tree. Content identical; only layout changes.
- **Variables** — non-secret variables (`f/memory/*` prompts and parameters) become variable files whose values are the current `prompts/*.md` texts. The prompt files stay the editable source; a small generator step (or plain file inclusion, if the sync format allows it) keeps them in sync — to be settled during implementation.
- **Schedules** — synced with `--include-schedules`. The schedule definitions MUST carry the fixes applied live on 2026-08-30: `no_flow_overlap: true` on all three, and taskmine cron `0 0 4 * * 7` (this Windmill cron parser rejects Sunday as `0`). Otherwise the next push regresses them.
- **App** — the approval inbox is a full-code React app currently deployed by pasting files into the in-browser editor (APP.md). Open question ①: whether `wmill sync` round-trips full-code apps faithfully (bundled `raw_app.yaml` + files + npm dependencies). First implementation step: `wmill sync pull` against the live workspace and inspect what the app serializes to. If fidelity is bad, the app keeps a separate deploy path (app API PUT with a locally built bundle) inside the same `just` recipe — still scripted, just not via sync.

Migration sequence (one-time): `wmill sync pull` from the live workspace into the new tree → diff against the repo's flow YAMLs and prompts → reconcile (live wins where it was hand-tuned, repo wins where live is stale) → commit as canonical → delete the old `flows/` layout and the paste instructions from `setup.md`/`APP.md`.

## Layer 4 — secrets

`CLAUDE_CODE_OAUTH_TOKEN` and the Windmill admin token never enter the repo, `wmill.yaml` state, or any synced file (`skipSecrets` stays true). At bootstrap, a `just` recipe reads them from the GNOME keyring (`secret-tool`, same convention as the `secrets` skill) and pushes the one workspace secret via `wmill variable`. Machines without the keyring entry fail loudly at that step.

## Layer 5 — one command + drift gate

- `just bootstrap host=<ssh-host>`: composed of small recipes per justfile rules — `compose-up` → `workspace-init` → `sync-push` → `secrets-push` → `verify`. Each is independently runnable; `verify` asserts via API: 3 schedules enabled with `no_flow_overlap=true`, 3 flows present, app present, both worker groups pinging.
- Drift gate in CI: `wmill sync pull --dry-run` (or `sync push --dry-run`) against the live workspace must show an empty diff; non-empty fails the job. This is what makes the repo the source of truth permanently — UI hot-fixes must be pulled back into the repo or they break CI. Open question ②: CI runner needs network reach to the Pi (Tailscale/VPN or a self-hosted runner on the LAN); pick during implementation.

## Explicitly rejected

- Terraform with the docker provider for layer 1: duplicates compose for zero gain.
- Terraform `local-exec` wrapping `wmill`: state without value; `wmill sync` already diffs.
- Windmill EE git-sync (auto push-on-deploy from UI): EE-only feature; CE stays, direction here is repo→instance anyway.

## Acceptance criteria

- Fresh box + keyring secrets + (at most) one manual first-admin login → `just bootstrap` → approval inbox reachable, schedules armed with overlap guard, a manual `f/memory/intake` trigger completes on the correct workers.
- `wmill sync pull --dry-run` clean right after bootstrap.
- No workspace object exists only in the UI: deleting the workspace and re-running bootstrap reproduces it (minus run history).
