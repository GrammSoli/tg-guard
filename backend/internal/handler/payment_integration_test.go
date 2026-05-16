//go:build integration

package handler_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"gorm.io/gorm"

	"github.com/subguard/backend/internal/config"
	"github.com/subguard/backend/internal/handler"
	"github.com/subguard/backend/internal/model"
	"github.com/subguard/backend/internal/testhelper"
)

const cryptoTestToken = "00000:CRYPTO_PAY_TEST_TOKEN_FOR_INTEGRATION"

// newPaymentApp wires HandleCryptoWebhook on POST /webhook/crypto.
// The handler doesn't need a Telegram bot reference for the
// signature/idempotency/DB paths — the congratulation send is guarded
// by `if h.bot != nil` and stays a no-op here.
func newPaymentApp(t *testing.T) (*fiber.App, *model.User, *gorm.DB) {
	t.Helper()
	db := testhelper.NewPostgres(t)

	// Seed one user so the webhook has someone to flip premium on.
	user := model.User{
		TelegramID: 999111,
		FirstName:  "Pay",
		Locale:     "en",
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}

	cfg := &config.Config{
		CryptoPayToken:  cryptoTestToken,
		CryptoPayAPIURL: "https://testnet-pay.crypt.bot/api/",
	}
	h := handler.NewPaymentHandler(db, cfg, nil)

	app := fiber.New()
	app.Post("/webhook/crypto", h.HandleCryptoWebhook)
	return app, &user, db
}

// sendWebhook fires a signed POST with the given JSON body. Returns
// (status, response body bytes).
func sendWebhook(t *testing.T, app *fiber.App, body []byte, signature string) int {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/webhook/crypto", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("crypto-pay-api-signature", signature)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 30 * time.Second})
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer resp.Body.Close()
	return resp.StatusCode
}

// buildInvoicePaidBody serialises the JSON shape Crypto Pay POSTs to
// the webhook: an "invoice_paid" update with a nested invoice payload.
func buildInvoicePaidBody(t *testing.T, invoiceID int64, payload, amount string) []byte {
	t.Helper()
	invoice := map[string]any{
		"invoice_id": invoiceID,
		"status":     "paid",
		"amount":     amount,
		"fiat":       "USD",
		"paid_at":    "2026-05-16T12:00:00Z",
		"payload":    payload,
	}
	invoiceJSON, err := json.Marshal(invoice)
	if err != nil {
		t.Fatalf("marshal invoice: %v", err)
	}
	update := map[string]any{
		"update_type":  "invoice_paid",
		"update_id":    1001,
		"request_date": "2026-05-16T12:00:01Z",
		"payload":      json.RawMessage(invoiceJSON),
	}
	body, err := json.Marshal(update)
	if err != nil {
		t.Fatalf("marshal update: %v", err)
	}
	return body
}

func TestCryptoWebhook_InvalidSignature_Returns401(t *testing.T) {
	app, _, _ := newPaymentApp(t)
	body := buildInvoicePaidBody(t, 1, "premium_crypto_lifetime_1", "10")
	status := sendWebhook(t, app, body, "deadbeef")
	if status != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", status)
	}
}

func TestCryptoWebhook_NonPaidUpdate_Returns200_NoOp(t *testing.T) {
	app, _, h := newPaymentApp(t)
	// Build a non-"invoice_paid" update.
	update := map[string]any{
		"update_type":  "invoice_created",
		"update_id":    2,
		"request_date": "2026-05-16T12:00:00Z",
		"payload":      json.RawMessage(`{"invoice_id":5,"status":"active","amount":"10","fiat":"USD","paid_at":"","payload":"premium_crypto_month_1"}`),
	}
	body, _ := json.Marshal(update)
	sig := testhelper.SignCryptoPayload(cryptoTestToken, body)

	if status := sendWebhook(t, app, body, sig); status != http.StatusOK {
		t.Errorf("status = %d, want 200", status)
	}
	// No donation row should have been created.
	var n int64
	h.Model(&model.Donation{}).Count(&n)
	if n != 0 {
		t.Errorf("donation count = %d, want 0", n)
	}
}

func TestCryptoWebhook_HappyPath_ActivatesPremium(t *testing.T) {
	app, user, h := newPaymentApp(t)

	body := buildInvoicePaidBody(t, 42, fmt.Sprintf("premium_crypto_lifetime_%d", user.ID), "10")
	sig := testhelper.SignCryptoPayload(cryptoTestToken, body)

	if status := sendWebhook(t, app, body, sig); status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}

	var refreshed model.User
	if err := h.First(&refreshed, user.ID).Error; err != nil {
		t.Fatalf("reload user: %v", err)
	}
	if !refreshed.IsDonator {
		t.Error("IsDonator = false, want true after invoice_paid")
	}
	if refreshed.PremiumExpiresAt != nil {
		t.Errorf("PremiumExpiresAt = %v, want nil for lifetime plan", refreshed.PremiumExpiresAt)
	}

	var donation model.Donation
	if err := h.Where("telegram_payment_charge_id = ?", "crypto_42").First(&donation).Error; err != nil {
		t.Fatalf("donation not persisted: %v", err)
	}
	if donation.Amount != 1000 {
		t.Errorf("amount cents = %d, want 1000 (10.00 USD)", donation.Amount)
	}
}

// TestCryptoWebhook_DuplicateInvoice_NoDoubleCharge is the headline
// test: Crypto Pay retries on slow / non-200 responses, and two pods
// behind a load balancer can race on the same delivery. The handler
// uses INSERT … ON CONFLICT DO NOTHING on the unique charge_id, so
// the second call must be a silent no-op — same donation row count,
// same amount, no second "thank you" Telegram message.
func TestCryptoWebhook_DuplicateInvoice_NoDoubleCharge(t *testing.T) {
	app, user, h := newPaymentApp(t)

	body := buildInvoicePaidBody(t, 99, fmt.Sprintf("premium_crypto_month_%d", user.ID), "5")
	sig := testhelper.SignCryptoPayload(cryptoTestToken, body)

	// First delivery: activates premium.
	if status := sendWebhook(t, app, body, sig); status != http.StatusOK {
		t.Fatalf("first delivery status = %d, want 200", status)
	}
	// Second delivery (Telegram retry): must NOT create another donation.
	if status := sendWebhook(t, app, body, sig); status != http.StatusOK {
		t.Fatalf("retry status = %d, want 200 (idempotent)", status)
	}

	var donationCount int64
	if err := h.Model(&model.Donation{}).Where("telegram_payment_charge_id = ?", "crypto_99").Count(&donationCount).Error; err != nil {
		t.Fatalf("count donations: %v", err)
	}
	if donationCount != 1 {
		t.Errorf("donation rows for charge_id crypto_99 = %d, want 1 (double-charge guard broken)", donationCount)
	}

	// PremiumExpiresAt should be ~1 month out and stable across the retry.
	var refreshed model.User
	if err := h.First(&refreshed, user.ID).Error; err != nil {
		t.Fatalf("reload user: %v", err)
	}
	if refreshed.PremiumExpiresAt == nil {
		t.Fatal("PremiumExpiresAt = nil, want ~1 month out for month plan")
	}
}

func TestCryptoWebhook_MalformedPayload_Returns200_NoOp(t *testing.T) {
	app, _, h := newPaymentApp(t)

	body := buildInvoicePaidBody(t, 7, "not_a_valid_payload", "10")
	sig := testhelper.SignCryptoPayload(cryptoTestToken, body)

	// Crypto Pay would otherwise retry on non-200; we accept and log.
	if status := sendWebhook(t, app, body, sig); status != http.StatusOK {
		t.Errorf("status = %d, want 200 (silent no-op on malformed payload)", status)
	}
	var n int64
	h.Model(&model.Donation{}).Count(&n)
	if n != 0 {
		t.Errorf("donation count = %d, want 0", n)
	}
}

func TestCryptoWebhook_UnknownUserID_Returns200_NoOp(t *testing.T) {
	app, _, h := newPaymentApp(t)

	// Payload points at a user ID that doesn't exist.
	body := buildInvoicePaidBody(t, 8, "premium_crypto_lifetime_999999999", "10")
	sig := testhelper.SignCryptoPayload(cryptoTestToken, body)

	if status := sendWebhook(t, app, body, sig); status != http.StatusOK {
		t.Errorf("status = %d, want 200 (silent no-op on unknown user)", status)
	}
	var n int64
	h.Model(&model.Donation{}).Count(&n)
	if n != 0 {
		t.Errorf("donation count = %d, want 0 (no row should be written for unknown user)", n)
	}
}

func TestCryptoWebhook_AmountRoundsCorrectly(t *testing.T) {
	app, user, h := newPaymentApp(t)

	// "0.10" via ParseFloat is 0.0999999…; the handler's math.Round
	// must save 10 cents, not 9. Audit-Low regression guard.
	body := buildInvoicePaidBody(t, 11, fmt.Sprintf("premium_crypto_lifetime_%d", user.ID), "0.10")
	sig := testhelper.SignCryptoPayload(cryptoTestToken, body)
	if status := sendWebhook(t, app, body, sig); status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}

	var donation model.Donation
	if err := h.Where("telegram_payment_charge_id = ?", "crypto_11").First(&donation).Error; err != nil {
		t.Fatalf("donation not persisted: %v", err)
	}
	if donation.Amount != 10 {
		t.Errorf("amount cents = %d, want 10 (IEEE-754 drift: 0.10 → 9 cents bug)", donation.Amount)
	}
}
