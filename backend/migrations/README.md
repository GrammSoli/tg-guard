# Database migrations

Versioned SQL migrations applied with [golang-migrate](https://github.com/golang-migrate/migrate).
They run **out-of-band** (a deploy step or manual command), never on app boot —
so rolling deploys on multiple nodes don't race on `ALTER TABLE`.

## File naming

```
000001_baseline.up.sql      000001_baseline.down.sql
000002_<change>.up.sql      000002_<change>.down.sql
```

Create a new pair with: `make migrate-create name=add_some_column`

## Status

The framework is wired up (Makefile targets, this directory), but the
**baseline migration (`000001_*`) has not been generated yet** — it must be
produced from a real production schema dump. See
[`docs/MIGRATIONS.md`](../../docs/MIGRATIONS.md) for the adoption procedure.

Until the baseline cutover lands, schema changes still go through the ad-hoc
`runMigration(...)` block in `backend/cmd/server/main.go`. Do **not** mix the
two: once `000001_baseline` exists and is `force`-marked on every environment,
the `runMigration` block is removed in the same PR.
