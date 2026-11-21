# memory-server deploy

Set `MEMORY_TOKEN` and start: `docker compose up -d memory-server`.

## Remote deploy (published image)

Images are published to GHCR by the `publish` workflow on push to
`main`: `ghcr.io/orfeo42/claude-memory-sync-server` and
`ghcr.io/orfeo42/claude-memory-sync-agent`. On the remote box only
`deploy/server/compose.yaml` and Docker are needed, no repo checkout
or build required.

If the package is private, log in first:
`docker login ghcr.io -u <user>` with a PAT that has `read:packages`.

Deploy or update with:
`docker compose pull memory-server && docker compose up -d memory-server`.

## Synthesis service (synth)

The LLM synthesis passes (intake judgment, daily condense/dedup +
hot-cache, weekly task mining, proposal apply) run in the `synth`
service: an in-process scheduler plus flow runner, the `claude` CLI,
and a web UI for approving proposals. It talks to `pipeline-api`
(proposals, run history, settings and prompt overrides live there) and
reads/writes the memory repos on the shared `memory-data` volume. The
image is `ghcr.io/orfeo42/claude-memory-sync-synth`; design and cutover
from Windmill: `.docs/13-drop-windmill.md`.

Start: `docker compose pull pipeline-api synth && docker compose up -d
pipeline-api synth`.

Environment (compose sets these; only the `.env` ones are yours):

- `PIPELINE_ADMIN_TOKEN` (`.env`, required): passed to synth as
  `PIPELINE_TOKEN`; `PIPELINE_URL` is `http://pipeline-api:8090`.
- `CLAUDE_CODE_OAUTH_TOKEN` (`.env`, optional): bootstrap fallback used
  only while the settings DB holds no token.
- `SYNTH_BIND` (`.env`, default `127.0.0.1`): host address of the UI
  port 8082. `TZ` (`.env`, default `Europe/Rome`): timezone of the cron
  schedules.
- Image defaults, rarely changed: `SYNTH_ADDR` (`:8082`), `SYNTH_WEB_DIR`
  (`/srv/web`), `MEMORY_REPOS_DIR` (`/data/repos`), `MEMORY_WORK_DIR`
  (`/data/work/canonical`), `MEMORY_LOCK` (`/data/state/canonical.lock`).

The container runs as uid 1000; `/data/work` and `/data/state` must be
owned by it.

UI access: the port is bound to `127.0.0.1:8082` on the server, so from
your machine run `ssh -L 8082:127.0.0.1:8082 <server>` and open
http://localhost:8082. There is NO authentication. Setting
`SYNTH_BIND=0.0.0.0` (or a LAN address) in `.env` exposes the UI on the
LAN: anyone there can approve proposals into `global/**` (synced to
every machine and loaded into every session), replace the Claude token
and start LLM runs. Keep the default unless the network is trusted.

Token: run `claude setup-token` on a machine with a browser
(subscription OAuth, valid 1 year) and paste it in the UI under
Settings. It is stored in pipeline-api, write-only (never returned by
the API) and wins over the env value; removing it falls back to
`CLAUDE_CODE_OAUTH_TOKEN`. Never set `ANTHROPIC_API_KEY` in the
environment: synth refuses to run any LLM step while it is present.

Schedules: each flow's cron expression (5 fields, in `TZ`) is editable
on the Runs page, with reset to default (intake `*/45 * * * *`,
daily-synth `0 3 * * *`, taskmine `0 4 * * 0`, proposal-apply
`*/10 * * * *`). The same page starts a flow on demand ("Run now") and
shows run history (kept 90 days) with per-step results.

Prompts: each LLM prompt is editable on the Prompts page, with version
history and reset to the default embedded in the image. Model and
other parameters are on the Settings page.

## Backup

Backup pushes the storage repo (branch `storage`) to a `origin` remote
already configured inside the `memory-data` volume, using a deploy key
with **write access** (repo Settings → Deploy keys → "Allow write
access"). One-time setup: `docker compose run --rm memory-backup git -C /data remote add origin <url>`.

Run the backup on demand or from host cron:
`MEMORY_BACKUP_SSH_KEY=~/.ssh/memory_deploy_key docker compose --profile backup run --rm memory-backup`.
