#!/usr/bin/env bash
# generate-baseline.sh — capture the production schema as the first
# golang-migrate baseline. See docs/MIGRATIONS.md for the surrounding
# 2-PR procedure; this script is step 1 of PR 2.
#
# Usage:
#   DATABASE_URL=postgres://user:pass@host:5432/db?sslmode=disable \
#     ./scripts/generate-baseline.sh
#
# What it does:
#   1. pg_dump --schema-only --no-owner --no-privileges into
#      backend/migrations/000001_baseline.up.sql
#   2. Strip migrate's own schema_migrations table if pg_dump included it
#   3. Sanity-check the dump contains the latest columns and indexes
#   4. Emit a no-op 000001_baseline.down.sql (baseline rollback = restore
#      a snapshot, not a migration)
#
# The script is intentionally idempotent: re-running overwrites the
# baseline files. If the dump fails any of the sanity checks, the
# files are left in place so the operator can inspect them, and the
# script exits non-zero.

set -euo pipefail

if [[ -z "${DATABASE_URL:-}" ]]; then
  echo "FAIL: DATABASE_URL must be set." >&2
  echo "  e.g. export DATABASE_URL='postgres://subguard:PASS@host:5432/subguard?sslmode=disable'" >&2
  exit 1
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
MIGRATIONS_DIR="${ROOT_DIR}/backend/migrations"
UP_FILE="${MIGRATIONS_DIR}/000001_baseline.up.sql"
DOWN_FILE="${MIGRATIONS_DIR}/000001_baseline.down.sql"

echo "▸ Dumping schema from ${DATABASE_URL%@*}@<redacted>"
pg_dump --schema-only --no-owner --no-privileges --dbname="${DATABASE_URL}" \
  > "${UP_FILE}"

# pg_dump sometimes includes the migrate-managed schema_migrations table
# when it was bootstrapped by a previous failed run. Strip it — migrate
# creates and owns this table itself, and the baseline shouldn't claim
# ownership of it.
if grep -q "CREATE TABLE.*schema_migrations" "${UP_FILE}"; then
  echo "▸ Stripping schema_migrations table from dump"
  # awk delete a CREATE TABLE block + its surrounding statements
  tmp="$(mktemp)"
  awk '
    /^CREATE TABLE.*schema_migrations/,/^\);/ { next }
    /^ALTER TABLE.*schema_migrations/ { next }
    /^INSERT INTO.*schema_migrations/ { next }
    { print }
  ' "${UP_FILE}" > "${tmp}"
  mv "${tmp}" "${UP_FILE}"
fi

echo "▸ Sanity-checking the dump"
fail=0
require_match() {
  local needle="$1"
  local label="$2"
  if ! grep -q -- "${needle}" "${UP_FILE}"; then
    echo "  ✗ ${label}: pattern '${needle}' not found"
    fail=1
  else
    echo "  ✓ ${label}"
  fi
}

# Columns the audit-pass added — if they're missing, the dump came
# from a stale environment.
require_match "price_stars_month_ru"     "users.price_stars_month_ru (plan split)"
require_match "last_billing_reset_at"    "shared_rooms.last_billing_reset_at (billing-reset idempotency)"
require_match "last_billing_reminder_at" "shared_rooms.last_billing_reminder_at (reminder idempotency)"
require_match "premium_expires_at"       "users.premium_expires_at"
require_match "timezone"                 "users/shared_rooms.timezone (per-room TZ)"
require_match "pause_notifications"      "app_settings.pause_notifications (kill switch)"

# Indexes the perf audit landed — if these are missing, alerts will
# fire as soon as the table grows.
require_match "idx_sub_due_unsent"       "subscriptions partial index"
require_match "idx_donations_charge_id"  "donations unique charge-id index"

if (( fail )); then
  echo
  echo "FAIL: dump is missing expected schema elements. Possible causes:"
  echo "  - DATABASE_URL points at a stale / old environment"
  echo "  - The backend hasn't booted yet against this DB, so the ad-hoc"
  echo "    runMigration() block in main.go hasn't applied recent ALTERs"
  echo "  - A migration was reverted manually outside of golang-migrate"
  echo
  echo "Inspect ${UP_FILE} and fix the source DB before re-running."
  exit 1
fi

cat > "${DOWN_FILE}" <<'EOF'
-- 000001_baseline.down.sql
-- Baseline rollback is a snapshot restore, not a migration.
-- See docs/BACKUP.md for the disaster-recovery procedure.
SELECT 1;
EOF

lines=$(wc -l < "${UP_FILE}" | tr -d ' ')
echo
echo "✓ Baseline written to ${UP_FILE} (${lines} lines)"
echo "✓ No-op down migration: ${DOWN_FILE}"
echo
echo "Next steps (from docs/MIGRATIONS.md PR 2):"
echo "  1. Commit both files."
echo "  2. On each environment (staging first), stamp the DB as already"
echo "     at version 1 (DO NOT run 'migrate up'):"
echo "       make migrate-baseline DATABASE_URL=<env-url>"
echo "       make migrate-version  DATABASE_URL=<env-url>   # expect: 1"
echo "  3. In the same PR, delete the runMigration() block in"
echo "     backend/cmd/server/main.go."
echo "  4. Add 'make migrate-up' as a deploy step (single runner,"
echo "     not per-pod)."
