# SubGuard operations runbook

What to do when something breaks in production. Keep this short and current.

**First three commands for any incident:**

```sh
docker compose ps                              # what's up / unhealthy
docker compose logs --tail 200 backend         # recent backend logs
curl -s localhost/health | jq                  # dependency status
```

`/health` returns `200 {"status":"ok"}` when healthy, or
`503 {"status":"degraded","db":"down","redis":"up"}` when a dependency is
unreachable. Sentry receives all 5xx responses and panics (`observability`
package) — check it first for a stack trace.

---

## 1. Payment webhook failing (user paid, no premium)

**Symptoms:** user reports a successful Stars/Crypto payment but premium is not
active; no congratulation DM.

**Check:**

```sh
docker compose logs backend | grep -iE 'webhook|crypto|payment|charge'
```

- **HMAC / secret mismatch** — Crypto Pay webhook verifies HMAC-SHA256 inside
  `handler.HandleCryptoWebhook`; Telegram webhook checks `WEBHOOK_SECRET`. A
  rotated secret or wrong `CRYPTO_PAY_API_TOKEN` rejects every delivery. Confirm
  `.env` matches the provider dashboard.
- **Provider status** — check the payment in the Crypto Pay dashboard / the
  Telegram payment. If the provider never sent the webhook, ask it to redeliver.
- **Rate limit** — `/webhook*` is capped at `WEBHOOK_RATE_LIMIT` (default
  600/min/IP). A `429` in logs means legitimate delivery was dropped; raise the
  limit and have the provider redeliver.

**Safe to retry:** webhook handlers are idempotent — they `INSERT ... ON
CONFLICT (telegram_payment_charge_id) DO NOTHING`. Redelivering the same payload
will not double-charge or double-activate. A duplicate logs `RowsAffected == 0`.

**Manual fix (last resort):** activate premium directly:

```sh
make db-shell
UPDATE users SET is_donator = true, premium_expires_at = now() + interval '1 month'
  WHERE telegram_id = <TG_ID>;
```

---

## 2. Broadcast partial-send (only some users got the message)

**Symptoms:** an admin broadcast reached a fraction of the base.

**Check:** `docker compose logs backend | grep -i broadcast` — the broadcast
goroutine logs progress and per-user send failures. Telegram rate-limits
(`429 retry_after`) and blocked-bot errors are expected for a slice of users.

- A `429` storm means the send pace is too high — the worker backs off
  (`workerutil` retry helpers); let it drain rather than re-triggering.
- A backend restart mid-broadcast **drops the rest** — there is no resume
  cursor. Re-running the broadcast is safe content-wise but will re-message
  users who already received it.

**Action:** wait for the goroutine to finish; only re-broadcast to the missed
segment if the tool supports segmentation, otherwise accept duplicates.

---

## 3. Database down

**Symptoms:** `/health` → `503 db:down`; 5xx spike; `pq:` / `dial tcp` errors.

**Check:**

```sh
docker compose ps postgres
docker compose logs --tail 100 postgres
docker compose exec postgres pg_isready -U subguard -d subguard
```

- **Container down** — `docker compose up -d postgres`; the backend reconnects
  automatically (connection pool retries).
- **Connection pool exhausted** — logs show `too many clients` / handlers
  hanging. Tune `DB_MAX_OPEN_CONNS` (env); check for a slow query holding
  connections (`SELECT * FROM pg_stat_activity WHERE state != 'idle'`).
- **Disk full** — `df -h`; the `pgdata` volume filling stops all writes.

k8s liveness probes hit `/health` and will recycle a pod whose DB is
unreachable — expected. Fix Postgres, pods recover on their own.

---

## 4. Redis down

**Symptoms:** `/health` → `503 redis:down`; currency conversion shows stale or
default rates.

**Check:**

```sh
docker compose ps redis
docker compose exec redis redis-cli ping     # expect PONG
```

Redis holds the FX-rate cache only — subscriptions, rooms and payments are
unaffected. The currency worker re-populates rates on its next tick once Redis
is back. `docker compose up -d redis` to restore. Note Redis runs with
`allkeys-lru` at a 64 MB cap, so eviction under memory pressure is normal.

---

## 5. 5xx spike

**Symptoms:** Sentry error rate climbs; users see failures.

**Triage:**

1. **Sentry** — group by the error; a single stack trace usually points at one
   handler. Panics are captured with the request path (`http:<path>`) and user.
2. **`/health`** — rule out a dependency outage (incidents 3 & 4).
3. **Recent deploy** — `git log` the last release. If the spike started at a
   deploy, roll back to the previous image:
   ```sh
   docker compose pull backend && docker compose up -d backend   # or pin the prior tag
   ```
4. **`recover` middleware** logs every panic with a full stack trace
   (`grep '\[recover\]'`) — the handler never crashes the process, but a
   recurring panic means a real bug.

**Rollback is the default** for a deploy-correlated spike — investigate after
traffic is stable, not during.

---

## Escalation

- Owner Telegram IDs: see `ADMIN_TELEGRAM_IDS` in `.env`.
- Sentry project: SubGuard backend (DSN in `.env`).
- Keep this file updated after every real incident — add the symptom and the fix.
