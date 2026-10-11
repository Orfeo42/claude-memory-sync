# Phase 12 — windmill-db Postgres 16 → 18

Status: **plan, rehearsed, not executed on the Pi** (2026-10-11). Follows the official Windmill guide (<https://www.windmill.dev/docs/advanced/self_host/postgres_18_upgrade>), adapted to this deployment.

## Rehearsal (2026-10-11)

Steps 3, 5–8 run on a workstation against a live `pg_dumpall` of the Pi cluster, restored into a throwaway `postgres:18-alpine` (18.6), then the Pi's exact Windmill image (`ghcr.io/windmill-labs/windmill@sha256:f5e3a77b…`, `CE v1.830.0-6-g0758bdc46b`) booted against it:

- Dump 354 MB (text, larger than the 110 MB on-disk data); one `CREATE DATABASE windmill`; trailer present.
- Restore: the only error was `ERROR:  role "windmill" already exists`.
- Baseline diff: databases, 361 policies, 5 roles, superuser, triggers identical; completed-job count off by one only because the rehearsal did not stop the live Windmill (step 1 freezes it in the real run).
- The restore carries the production password hash for role `windmill`, so the Pi's existing `WINDMILL_DB_PASSWORD` keeps working; nothing in `.env` changes.
- Windmill booted, its startup migration was a no-op, and the real API token listed the 3 flows, 3 schedules (`no_flow_overlap: true` via `schedules/get`; `schedules/list` omits the field) and the pre-migration run history — auth and RLS intact.
- Disk: the dump plus a second ~200 MB volume fit easily (70 GB free on the Pi).

## Why a plan and not a tag bump

`windmill-db` runs `postgres:16-alpine` with its data in volume `memory-server_windmill-db-data` mounted at `/var/lib/postgresql/data`. The Postgres 18 image keeps its cluster in a versioned subdirectory (`/var/lib/postgresql/18/docker`) and expects the volume at `/var/lib/postgresql`. Only changing the tag makes Postgres 18 initialise a fresh, empty cluster next to the old files: Windmill then boots against an empty database (all flows, variables, schedules, app, run history gone from view) without any error. A major upgrade needs either `pg_upgrade` or dump + restore; the cluster is small (110 MB data, 182 MB on disk), so dump + restore is the simplest and the upstream-recommended path. Upstream Windmill's own compose now ships `postgres:18`; 16 stays supported until November 2028, so this is not urgent.

## Differences from the upstream guide

- Superuser is `windmill` (`POSTGRES_USER: windmill`), not `postgres`: every `-U postgres` becomes `-U windmill`, and the one expected restore error is `role "windmill" already exists`.
- The old volume is **kept** instead of deleted: the new cluster goes into a new volume `windmill-db-data-18`. Rollback = revert two compose lines.
- Only the Windmill services stop. `memory-server` (git hub, agents keep syncing) and `pipeline-db`/`pipeline-api` stay up.
- No Postgres triggers, replication slots or publications exist, so the guide's trigger re-save step does not apply.

## Baseline (captured 2026-10-11, re-capture right before migrating)

| check | query | value |
| --- | --- | --- |
| databases | `select datname from pg_database where datallowconn and datname not in ('postgres','template1')` | `windmill` |
| RLS policies | `select count(*) from pg_policies` | 361 |
| roles | `select count(*) from pg_roles where rolname not like 'pg\_%'` | 5 |
| superuser | `select rolname from pg_roles where rolsuper` | `windmill` |
| completed jobs | `select count(*) from v2_job_completed` | 7059 |
| suspended jobs | `select count(*) from v2_job_queue where suspend > 0` | 0 |
| Postgres triggers | `select count(*) from postgres_trigger` | 0 |

`pg_dumpall` (not `pg_dump`) is mandatory: Windmill's RLS policies depend on cluster-level roles (`windmill_user`, `windmill_admin`, `custom_instance_*`), and a single-database dump silently drops policies whose grantee role is missing.

## Procedure (on the Pi, in `~/apps/memory-server`)

Prerequisite: run it in its own maintenance window, not together with the phase 11 rollout. Expected Windmill downtime: about 10 minutes; one intake tick may be skipped.

1. **Quiesce.** Wait until no job is running (`select count(*) from v2_job_queue where running` = 0) and confirm 0 suspended jobs. Then stop only Windmill:
    ```sh
    docker compose stop windmill-server windmill-worker-llm-steps windmill-worker-ui-backend
    ```
2. **Baseline.** Save the table's queries as `backups/baseline.sql` and run them against the old cluster:
    ```sql
    select string_agg(datname, ',' order by datname) from pg_database where datallowconn and datname not in ('postgres','template1');
    select count(*) from pg_policies;
    select count(*) from pg_roles where rolname not like 'pg\_%';
    select string_agg(rolname, ',' order by rolname) from pg_roles where rolsuper;
    select count(*) from v2_job_completed;
    select count(*) from v2_job_queue where suspend > 0;
    select count(*) from postgres_trigger;
    ```
    ```sh
    mkdir -p backups && chmod 700 backups
    docker compose exec -T windmill-db psql -U windmill -d windmill -At < backups/baseline.sql > backups/baseline-old.txt
    ```
3. **Dump the whole cluster** (contains password hashes — keep it private):
    ```sh
    docker compose exec -T windmill-db pg_dumpall -U windmill > backups/windmill-cluster-$(date +%F).sql
    chmod 600 backups/windmill-cluster-*.sql
    grep '^CREATE DATABASE' backups/windmill-cluster-*.sql
    tail -n 3 backups/windmill-cluster-*.sql
    ```
    Expect exactly one `CREATE DATABASE windmill` and the trailer `-- PostgreSQL database cluster dump complete`. Stop here if either is missing.
4. **Switch compose** (`deploy/server/compose.yaml`, mirrored to the Pi):
    ```diff
       windmill-db:
    -    image: postgres:16-alpine
    +    image: postgres:18-alpine
         volumes:
    -      - windmill-db-data:/var/lib/postgresql/data
    +      - windmill-db-data-18:/var/lib/postgresql
     ...
     volumes:
       memory-data:
       windmill-db-data:
    +  windmill-db-data-18:
    ```
5. **Start the new, empty cluster** and wait for the healthcheck:
    ```sh
    docker compose up -d --wait windmill-db
    docker compose exec -T windmill-db postgres --version
    ```
6. **Restore:**
    ```sh
    docker compose exec -T windmill-db psql -U windmill -d postgres -c 'DROP DATABASE windmill'
    docker compose exec -T windmill-db psql -U windmill -d postgres < backups/windmill-cluster-*.sql > backups/restore.log 2>&1
    grep -i '^ERROR' backups/restore.log
    ```
    The `DROP DATABASE` lets the dump's `CREATE DATABASE windmill` run with its original settings. `psql` does not stop on errors: the only acceptable line is `ERROR:  role "windmill" already exists`. Anything else = partial restore → rollback.
7. **Statistics:**
    ```sh
    docker compose exec -T windmill-db vacuumdb -U windmill --all --analyze-in-stages
    ```
8. **Verify the new cluster:**
    ```sh
    docker compose exec -T windmill-db psql -U windmill -d windmill -At < backups/baseline.sql > backups/baseline-new.txt
    diff backups/baseline-old.txt backups/baseline-new.txt
    ```
    The diff must be empty.
9. **Start Windmill:** `docker compose up -d`. Then: log in; Workers page shows both worker groups; flows, variables, schedules and the approval app are present; run `f/memory/intake` manually and see it succeed; open an old run from before the migration.
10. **After 7 clean days:** remove `windmill-db-data:` from `compose.yaml`, `docker volume rm memory-server_windmill-db-data`, and delete `backups/windmill-cluster-*.sql` (or move it off the Pi).

## Rollback

Any time before step 10: `docker compose stop windmill-server windmill-worker-llm-steps windmill-worker-ui-backend windmill-db`, revert the two compose lines from step 4, `docker compose up -d`. The untouched 16 volume comes back as it was at step 1; only run history written after the migration is lost. Never revert just the image tag while keeping the new mount.

## Not changed

`pipeline-db` already runs `postgres:18-alpine` with the 18 mount layout. The repo's `compose.yaml` keeps `windmill-db` on 16 until this procedure is executed: the step 4 diff must land on the Pi only as part of running it.
