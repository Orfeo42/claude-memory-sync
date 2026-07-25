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

Backup pushes the storage repo (branch `storage`) to a `origin` remote
already configured inside the `memory-data` volume, using a read-only
deploy key. One-time setup: `docker compose run --rm memory-backup git -C /data remote add origin <url>`.

Run the backup on demand or from host cron:
`MEMORY_BACKUP_SSH_KEY=~/.ssh/memory_deploy_key docker compose --profile backup run --rm memory-backup`.
