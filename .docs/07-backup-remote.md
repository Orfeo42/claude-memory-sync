# Phase 7 — GitHub backup remote (future, context only)

Not planned yet. User parked this on 2026-07-26 ("future step"). This
doc exists so a future session can execute it without re-deriving the
following facts.

## Goal

Enable the existing `memory-backup` compose service (profile `backup`,
`deploy/server/compose.yaml`) to push the storage repo (branch
`storage`) from the Pi to a private GitHub repo. GitHub is backup only
— never the sync path (README ground rule).

## Known gotchas (discovered 2026-07-26, fix during implementation)

1. **README one-time `remote add` command is broken.** The service has
   a fixed entrypoint `git -C /data push origin storage`, so
   `docker compose run --rm memory-backup git -C /data remote add ...`
   appends the args to the push command instead of replacing it. The
   working form needs an entrypoint override:
   `docker compose run --rm --entrypoint git memory-backup -C /data remote add origin <url>`.
   Fix the README line when implementing.
2. **No `known_hosts` in alpine/git.** First SSH push fails with
   "Host key verification failed". Fix: add to the service
   `environment`: `GIT_SSH_COMMAND: ssh -o StrictHostKeyChecking=accept-new`
   (pins the host key on first use).
3. **Deploy key must have WRITE access** — GitHub repo Settings →
   Deploy keys → check "Allow write access". (README used to say
   read-only; wording already corrected.)

## Setup steps when executed

1. On the Pi: `ssh-keygen -t ed25519 -f ~/.ssh/memory_deploy_key -N "" -C memory-backup`.
2. Create/choose the private backup repo on GitHub; add the public key
   as a deploy key with write access.
3. Add `GIT_SSH_COMMAND` env to the `memory-backup` service (gotcha 2).
4. One-time: `docker compose run --rm --entrypoint git memory-backup -C /data remote add origin git@github.com:<owner>/<repo>.git`.
5. Test: `MEMORY_BACKUP_SSH_KEY=~/.ssh/memory_deploy_key docker compose --profile backup run --rm memory-backup`.
6. Recurring: host cron or systemd timer on the Pi running step 5's
   command daily (project convention from phase 1: timers, not cron).

## Open questions

- Which repo is the backup target (dedicated private repo name)?
- Daily timer on the Pi host vs a loop service like the synthesizer —
  pick when implementing (synthesizer pattern now exists as precedent).
