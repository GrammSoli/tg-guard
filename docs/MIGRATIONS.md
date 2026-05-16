# Migration framework adoption (golang-migrate)

## Why

Today the schema is patched by an ad-hoc `runMigration(...)` block in
`backend/cmd/server/main.go` (lines ~107-180) that runs `ALTER TABLE ... IF NOT
EXISTS` on **every app boot**. Problems:

- Rolling deploys on 2+ nodes race on `AccessExclusiveLock` during `ALTER TABLE`.
- No rollback path — `DROP COLUMN`, `RENAME`, data backfills have nowhere to live.
- No version record — you cannot tell which schema a given DB is on.

golang-migrate replaces it with versioned SQL files applied **out-of-band**.

## Status / scope

This is a **two-PR change**:

- **PR 1 (this one):** scaffolding only — `backend/migrations/`, Makefile
  targets, this doc. No behaviour change; `runMigration` still runs.
- **PR 2 (separate, rehearsed on staging):** generate the baseline, `force`-mark
  every environment, delete the `runMigration` block. Steps below.

> ⚠️ Do not do PR 2 without a staging rehearsal. A wrong baseline version
> re-runs DDL against production.

## Prerequisites

```sh
brew install golang-migrate          # macOS
export DATABASE_URL="postgres://subguard:PASS@HOST:5432/subguard?sslmode=disable"
```

## PR 2 — baseline adoption procedure

### 1. Generate the baseline from the REAL production schema

The baseline must equal whatever production currently has — **after** the
current backend (with the `runMigration` block) has booted at least once, so
every column and partial index exists.

```sh
pg_dump --schema-only --no-owner --no-privileges \
  -U subguard -d subguard > backend/migrations/000001_baseline.up.sql
```

Verify the dump contains the latest columns before trusting it, e.g.:

```sh
grep -c price_stars_month_ru backend/migrations/000001_baseline.up.sql   # expect 1
grep -c last_billing_reset_at backend/migrations/000001_baseline.up.sql  # expect 1
grep -c idx_sub_due_unsent     backend/migrations/000001_baseline.up.sql # expect 1
```

> A dump taken from a stale environment (e.g. a local container running an old
> image) will have diverged columns such as `price_stars_ru` instead of
> `price_stars_month_ru`. Always dump the canonical production DB.

Remove the `schema_migrations` table from the dump if `pg_dump` included it —
migrate manages that table itself.

The `down` migration is a no-op (rolling the baseline back = restore a snapshot,
see `docs/BACKUP_VERIFICATION.md`):

```sql
-- 000001_baseline.down.sql
-- Baseline rollback is a snapshot restore, not a migration. Intentionally empty.
SELECT 1;
```

### 2. Stamp existing databases (prod + staging)

The schema already exists, so do **not** run `migrate up` for `000001`. Mark it
applied instead:

```sh
make migrate-baseline DATABASE_URL=...    # == migrate force 1
make migrate-version  DATABASE_URL=...    # expect: 1
```

### 3. Rehearse on staging

On a fresh DB restored from a prod snapshot:

```sh
make migrate-baseline DATABASE_URL=<staging>
make migrate-version  DATABASE_URL=<staging>     # 1
```

Then create a throwaway `000002` and confirm `migrate up` / `migrate down 1`
work. Diff `pg_dump --schema-only` of a from-scratch `migrate up` against the
prod dump — they must be identical.

### 4. Remove the ad-hoc block

In the same PR, delete the `runMigration(...)` block in
`backend/cmd/server/main.go` (~lines 107-180). Keep the test-mode
`AutoMigrate` (it only runs when `APP_ENV=test` / `RUN_MIGRATIONS=1`).

### 5. Wire migrations into deploy

Run `make migrate-up` as a deploy step **before** the new app version starts,
from a single runner (not per-pod).

## Day-to-day

```sh
make migrate-create name=add_user_locale   # new 000NNN pair
# edit the generated .up.sql / .down.sql
make migrate-up                            # apply
make migrate-down                          # roll back one step
```
