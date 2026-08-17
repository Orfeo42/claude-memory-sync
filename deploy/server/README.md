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

## Synthesis pipeline (Windmill)

The LLM synthesis passes (intake judgment, daily condense/dedup +
hot-cache, weekly task mining) run as Windmill flows on the
`windmill-server`/`windmill-worker` services — setup runbook:
`deploy/windmill/setup.md`. The worker image bundles `memoryctl`, git,
and the `claude` CLI.

One-time setup: run `claude setup-token` on a machine with a browser
(subscription OAuth, valid 1 year), put the printed token in `.env` as
`CLAUDE_CODE_OAUTH_TOKEN` (required by `windmill-worker`). Never set
`ANTHROPIC_API_KEY` in the environment — it would take precedence and
bill API credits; every flow step refuses to run if it is present.
`WINDMILL_DB_PASSWORD` must also be set in `.env`.

Start: `docker compose up -d windmill-db windmill-server
windmill-worker`. The Windmill UI is on port 8081; flow schedules
replace the old `SYNTH_INTERVAL`/`INTAKE_INTERVAL` env tuning.

## Backup

Backup pushes the storage repo (branch `storage`) to a `origin` remote
already configured inside the `memory-data` volume, using a deploy key
with **write access** (repo Settings → Deploy keys → "Allow write
access"). One-time setup: `docker compose run --rm memory-backup git -C /data remote add origin <url>`.

Run the backup on demand or from host cron:
`MEMORY_BACKUP_SSH_KEY=~/.ssh/memory_deploy_key docker compose --profile backup run --rm memory-backup`.
