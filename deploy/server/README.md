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

## Synthesizer (daily condense/dedup)

`memory-synthesizer` runs a daily LLM pass over `canonical/` memory:
merges duplicate entries, one git commit per project, revertable.

One-time setup: run `claude setup-token` on a machine with a browser
(subscription OAuth, valid 1 year), put the printed token in `.env` as
`CLAUDE_CODE_OAUTH_TOKEN`. Never set `ANTHROPIC_API_KEY` in the
environment — it would take precedence and bill API credits; the
service refuses to start if it is present. Optional: `SYNTH_INTERVAL`
(default `24h`), `SYNTH_MODEL` (default `sonnet`).

Start: `docker compose up -d memory-synthesizer`. Without the token
set, the other services still deploy fine; only the synthesizer exits.

## Backup

Backup pushes the storage repo (branch `storage`) to a `origin` remote
already configured inside the `memory-data` volume, using a deploy key
with **write access** (repo Settings → Deploy keys → "Allow write
access"). One-time setup: `docker compose run --rm memory-backup git -C /data remote add origin <url>`.

Run the backup on demand or from host cron:
`MEMORY_BACKUP_SSH_KEY=~/.ssh/memory_deploy_key docker compose --profile backup run --rm memory-backup`.
