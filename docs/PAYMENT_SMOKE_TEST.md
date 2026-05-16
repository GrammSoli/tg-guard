# Payment smoke test (pre-launch)

Run this after every deploy that touches payment code, and once before launch.
Both webhook paths (Crypto Pay + Telegram Stars) were rewritten to use
`INSERT ... ON CONFLICT` for idempotency — this checklist verifies activation
**and** that a redelivered webhook does not double-charge or double-activate.

Use a real test account that is not already a donator.

## A. Telegram Stars

Telegram Stars has no sandbox — use a real invoice at the lowest configured
price.

- [ ] Open the bot → trigger the premium / donate flow → pay with Stars.
- [ ] Payment succeeds in the Telegram UI.
- [ ] Within a few seconds: premium is active in the Mini App (badge ✨ shows,
      paywall lifted).
- [ ] Congratulation DM is received.
- [ ] DB check — `make db-shell`:
      ```sql
      SELECT is_donator, premium_expires_at FROM users WHERE telegram_id = <TG_ID>;
      SELECT count(*) FROM donations WHERE telegram_id = <TG_ID>;   -- expect 1
      ```

### Idempotency

- [ ] Redeliver the same `successful_payment` update (resend the webhook
      payload, or replay from logs).
- [ ] `donations` count for that user is **still 1** (not 2).
- [ ] Backend logs show a duplicate/`RowsAffected == 0` line, no error.
- [ ] No second DM, no double premium extension.

## B. Crypto Pay

Crypto Pay has a testnet. Point the backend at it without rebuilding:

```
CRYPTO_PAY_API_URL=<testnet endpoint>      # see handler/payment.go
CRYPTO_PAY_API_TOKEN=<testnet token>
```

- [ ] Trigger the crypto payment flow → complete a testnet invoice.
- [ ] `/webhook/crypto` returns 200 (check backend logs).
- [ ] Premium activates; congratulation DM received.
- [ ] DB check:
      ```sql
      SELECT is_donator, premium_expires_at FROM users WHERE telegram_id = <TG_ID>;
      SELECT telegram_payment_charge_id FROM donations WHERE telegram_id = <TG_ID>;
      -- charge id is prefixed `crypto_<invoice_id>`
      ```

### Idempotency

- [ ] Redeliver the same crypto webhook payload.
- [ ] HMAC still verifies, handler returns 200, `donations` count unchanged.
- [ ] No double activation.

## C. Cross-checks

- [ ] Repeat A or B with a second account set to the **other locale** (RU vs EN)
      — confirms the correct price column is read and the DM is localised.
- [ ] Confirm the rate limiter did not interfere — no unexpected `429` for
      legitimate webhook delivery in logs.

## Sign-off

| Test | Account | Result | Date |
|---|---|---|---|
| Stars activation |  |  |  |
| Stars idempotency |  |  |  |
| Crypto activation |  |  |  |
| Crypto idempotency |  |  |  |
