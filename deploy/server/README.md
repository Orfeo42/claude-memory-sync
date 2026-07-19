# memory-server deploy

Set `MEMORY_TOKEN` and start: `docker compose up -d memory-server`.

Backup pushes the storage repo (branch `storage`) to a `origin` remote
already configured inside the `memory-data` volume, using a read-only
deploy key. One-time setup: `docker compose run --rm memory-backup git -C /data remote add origin <url>`.

Run the backup on demand or from host cron:
`MEMORY_BACKUP_SSH_KEY=~/.ssh/memory_deploy_key docker compose --profile backup run --rm memory-backup`.
