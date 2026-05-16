# Backup verification (pre-launch)

A snapshot you have never restored is not a backup. Run this once before launch,
then on a schedule. It doubles as the staging environment for rehearsing the
migration baseline cutover (`docs/MIGRATIONS.md`).

## 1. Take a production snapshot

```sh
pg_dump -U subguard -d subguard -Fc -f subguard-$(date +%F).dump        # custom format
# or, if using managed Postgres, trigger/locate the provider snapshot.
```

## 2. Restore into a clean staging Postgres

Use a throwaway instance — never restore onto production.

```sh
docker run -d --name subguard-restore-test \
  -e POSTGRES_USER=subguard -e POSTGRES_PASSWORD=test \
  -e POSTGRES_DB=subguard -p 55432:5432 postgres:16-alpine

pg_restore -U subguard -d subguard -h localhost -p 55432 --no-owner \
  subguard-$(date +%F).dump
```

## 3. Verify integrity

- [ ] Restore completed with no errors.
- [ ] Row counts are plausible vs production:
      ```sql
      SELECT 'users', count(*) FROM users
      UNION ALL SELECT 'subscriptions', count(*) FROM subscriptions
      UNION ALL SELECT 'shared_rooms', count(*) FROM shared_rooms
      UNION ALL SELECT 'donations', count(*) FROM donations;
      ```
- [ ] Spot-check a recent row (e.g. newest `users.created_at`) matches prod.
- [ ] Schema check — `migrate version` (once the framework is adopted) or
      compare `pg_dump --schema-only` against production.

## 4. Boot the app against the restored DB

```sh
DATABASE_URL=postgres://subguard:test@localhost:55432/subguard?sslmode=disable \
RUN_MIGRATIONS=0 APP_ENV=test ./backend/bin/server
```

- [ ] App starts cleanly.
- [ ] `curl localhost:3001/health` (test-mode port) → `200 {"status":"ok"}`.

## 5. Tear down

```sh
docker rm -f subguard-restore-test
```

## 6. Record the result

| Date | Snapshot | Restore OK | Row counts OK | App boot OK | By |
|---|---|---|---|---|---|
|  |  |  |  |  |  |

If any step fails, the backup is **not** trustworthy — fix the backup process
before launch.
