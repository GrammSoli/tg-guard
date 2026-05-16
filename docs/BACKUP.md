# Backups — operator runbook

This is the on-call reference for routine backup operations: where they
live, how to restore one, and how to confirm the schedule is alive.
For the pre-launch one-shot verification procedure see
`BACKUP_VERIFICATION.md`.

## What's running

A `postgres-backup` sidecar lives in `docker-compose.yml` alongside the
Postgres service. It runs
[prodrigestivill/postgres-backup-local](https://github.com/prodrigestivill/docker-postgres-backup-local),
which is a thin wrapper around `pg_dump` with a cron-style scheduler
and a tiered retention policy. Dumps are gzip-compressed and land in
`./backups/` on the host.

### Schedule

Configured by environment variable (see `docker-compose.yml`):

| Var | Default | Meaning |
|---|---|---|
| `BACKUP_SCHEDULE` | `30 3 * * *` | Cron expression — daily 03:30 server time |
| `BACKUP_KEEP_DAYS` | 7 | Keep N most-recent daily dumps |
| `BACKUP_KEEP_WEEKS` | 4 | Keep N most-recent weekly dumps |
| `BACKUP_KEEP_MONTHS` | 6 | Keep N most-recent monthly dumps |

Tune in `.env` (do NOT edit `docker-compose.yml` directly for these).

### Layout on disk

```
backups/
├── daily/    subguard-YYYY-MM-DD.sql.gz
├── weekly/   subguard-YYYY-WW.sql.gz
└── monthly/  subguard-YYYY-MM.sql.gz
```

## Routine: confirm a fresh backup exists

Run daily as part of the morning check:

```sh
ls -lh backups/daily/ | tail -5
```

Expected: at least one file with today's or yesterday's date and a
non-zero size (a healthy `subguard` dump is in the low tens of MB
under 10k users; closer to hundreds of MB at scale).

If the newest file is more than 36 hours old, treat it as a backup
outage:

```sh
docker compose logs postgres-backup --tail=200
```

Common causes:
- Postgres container restarted during the dump window — next tick will
  recover on its own.
- Disk full on the host — `df -h .` to verify.
- Schedule misconfigured — `docker compose exec postgres-backup env | grep SCHEDULE`.

## Off-box the dumps

The sidecar writes to a local volume, which is gone if the VM is gone.
Copy dumps somewhere else daily — at the bare minimum, an `rsync` to
the operator workstation via `cron`:

```sh
# In root@host crontab, run after the backup window:
0 4 * * * rsync -avz /opt/tg-guard/backups/ operator@laptop:tg-guard-backups/
```

For a proper setup, mirror to an object store (S3, Backblaze B2,
Hetzner Storage Box) via `rclone sync ./backups remote:bucket/`.

## Restore drill: weekly, by hand

A backup that has never been restored is not a backup. Once a week,
do this:

```sh
# 1. Pick the most recent daily dump.
DUMP=$(ls -t backups/daily/*.sql.gz | head -1)

# 2. Spin up a throwaway Postgres.
docker run -d --name pg-restore-drill \
  -e POSTGRES_USER=subguard \
  -e POSTGRES_PASSWORD=drill \
  -e POSTGRES_DB=subguard \
  -p 55432:5432 \
  postgres:16-alpine

# Wait for it to be ready.
until docker exec pg-restore-drill pg_isready -U subguard >/dev/null 2>&1; do sleep 1; done

# 3. Restore.
gunzip -c "$DUMP" | docker exec -i pg-restore-drill psql -U subguard -d subguard

# 4. Sanity-check row counts.
docker exec pg-restore-drill psql -U subguard -d subguard -c "
  SELECT 'users' AS table, count(*) FROM users
  UNION ALL SELECT 'subscriptions', count(*) FROM subscriptions
  UNION ALL SELECT 'shared_rooms', count(*) FROM shared_rooms
  UNION ALL SELECT 'donations', count(*) FROM donations;"

# 5. Tear down.
docker rm -f pg-restore-drill
```

If row counts look wrong vs. production, the dump is corrupt — alert
and investigate before the next prod incident.

The CI workflow `.github/workflows/backup-restore-drill.yml` exercises
the *procedure* on a synthetic dataset weekly, but it cannot test
your actual backups — only this hand-drill does that.

## Disaster recovery — full restore onto a new VM

```sh
# On the new host, after docker compose up -d but BEFORE traffic hits:
docker compose stop backend           # ensure no writes during restore
docker compose exec -T postgres psql -U subguard -d subguard \
  -c "DROP SCHEMA public CASCADE; CREATE SCHEMA public;"
gunzip -c backups/daily/subguard-YYYY-MM-DD.sql.gz \
  | docker compose exec -T postgres psql -U subguard -d subguard
docker compose start backend
curl -fsS http://localhost/health | jq .
```

The `RUN_MIGRATIONS=0` flag prevents the boot path from racing the
restore. Boot once, verify, then re-enable the normal migration flow
on the next deploy.
