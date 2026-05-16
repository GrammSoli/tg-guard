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

Run the helper script — it captures the production schema, strips
migrate's own `schema_migrations` table from the dump, runs sanity
checks for the audit-era columns/indexes, and writes both
`000001_baseline.up.sql` and a no-op `.down.sql`:

```sh
DATABASE_URL='postgres://subguard:PASS@PROD_HOST:5432/subguard?sslmode=disable' \
  make migrate-generate-baseline
```

The script aborts if any expected column or index is missing — that's
the signal that `DATABASE_URL` points at a stale environment (e.g. a
local container running an old image). Always run against the canonical
production DB.

> The script must run **after** the current backend (with the
> `runMigration` block) has booted at least once against the target
> DB, so every column and partial index added by the audit pass
> actually exists.

The down migration is intentionally a no-op — rolling the baseline back
means restoring a snapshot, not running SQL. See `docs/BACKUP.md` for
the disaster-recovery procedure.

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
make migrate-status                        # current version (pre-deploy smoke)
```

## When `migrate-status` reports "dirty"

A previous `migrate up` or `down` died partway through — usually a SQL
error inside the migration, occasionally a process kill. The DB is in
an intermediate state and migrate refuses further ops until you
acknowledge it.

Procedure:
1. Look at the failed migration (`migrate-status` prints the version).
2. Reconcile the DB by hand — either finish the migration's intent or
   roll it back manually with `psql`.
3. Once the DB matches the version you reconciled to, force-stamp it:
   ```sh
   migrate -path backend/migrations -database "$DATABASE_URL" force <N>
   ```
4. Re-run `make migrate-status` — should be `version: <N>` with no
   "dirty" suffix.

If you can't easily reconcile, restore the most recent backup
(`docs/BACKUP.md`) — it's almost certainly faster than untangling
partial DDL.
