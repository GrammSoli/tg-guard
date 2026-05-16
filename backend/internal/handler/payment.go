package handler

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/gofiber/fiber/v3"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/subguard/backend/internal/config"
	"github.com/subguard/backend/internal/middleware"
	"github.com/subguard/backend/internal/model"
)

// cryptoPayClient is a dedicated HTTP client for outbound Crypto Pay API
// calls. http.DefaultClient has NO timeout, so a hung Crypto Pay endpoint
// would tie up the Fiber handler goroutine until the user's WebView gave
// up — the fiber request context bound via NewRequestWithContext only
// cancels when the client itself disconnects. A hard 15s ceiling keeps
// per-request latency bounded regardless of upstream behaviour.
var cryptoPayClient = &http.Client{Timeout: 15 * time.Second}

// PaymentHandler handles Premium payment endpoints (Stars + Crypto Pay).
type PaymentHandler struct {
	cfg *config.Config
	db  *gorm.DB
	bot *tgbot.Bot
}

func NewPaymentHandler(db *gorm.DB, cfg *config.Config, b *tgbot.Bot) *PaymentHandler {
	return &PaymentHandler{cfg: cfg, db: db, bot: b}
}

// planRequest is the JSON body both invoice endpoints accept.
type planRequest struct {
	Plan string `json:"plan"`
}

// normalizePlan coerces a request plan to one of the two valid values.
// Anything unrecognised (including empty) falls back to "lifetime" — the
// tier the frontend pre-selects, so a missing field is never a 400.
func normalizePlan(p string) string {
	if p == "month" {
		return "month"
	}
	return "lifetime"
}

// premiumExpiryFor returns the premium_expires_at value a plan grants:
// one month out for "month", nil (lifetime — never expires) otherwise.
func premiumExpiryFor(plan string) *time.Time {
	if plan == "month" {
		t := time.Now().UTC().AddDate(0, 1, 0)
		return &t
	}
	return nil
}

// parsePaymentPayload extracts (plan, userID) from an invoice payload.
// Format: "premium_<method>_<plan>_<userID>" — exactly 4 parts.
//
// A legacy 3-part shape ("premium_<method>_<userID>") used to be
// accepted and silently coerced to a LIFETIME grant. That defaulted
// any caller capable of crafting a 3-part payload to the most
// expensive tier — a small but real downgrade-resistance hole now
// that the codebase only emits 4-part payloads. Removed under audit
// Tier-1 #2; any in-flight legacy invoice (Stars invoices expire in
// hours, CryptoBot in days) is rejected as malformed, with the caller
// expected to repay through a freshly-issued 4-part invoice.
func parsePaymentPayload(payload, method string) (plan string, userID uint64, ok bool) {
	prefix := "premium_" + method + "_"
	if !strings.HasPrefix(payload, prefix) {
		return "", 0, false
	}
	parts := strings.Split(payload, "_")
	switch len(parts) {
	case 4: // premium / method / plan / uid
		plan = normalizePlan(parts[2])
		uid, err := strconv.ParseUint(parts[3], 10, 64)
		if err != nil {
			return "", 0, false
		}
		return plan, uid, true
	default:
		return "", 0, false
	}
}

// ── Stars ───────────────────────────────────────────────────────────

// CreateStarsInvoice generates a Telegram Stars payment link for the
// authenticated user. The price and copy are locale-split (ru/en) and
// read from AppSettings so the admin can adjust them in real-time from
// the in-bot panel.
//
// POST /api/v1/payments/stars
func (h *PaymentHandler) CreateStarsInvoice(c fiber.Ctx) error {
	user := middleware.UserFromCtx(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}
	if h.bot == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "bot unavailable"})
	}

	var settings model.AppSettings
	if err := h.db.FirstOrCreate(&settings, model.AppSettings{ID: 1}).Error; err != nil {
		log.Printf("[payment.stars] settings read error: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "settings unavailable"})
	}

	var body planRequest
	_ = c.Bind().JSON(&body) // empty / missing body → lifetime default
	plan := normalizePlan(body.Plan)

	locale := user.Locale
	if locale == "" {
		locale = "en"
	}
	isRu := strings.HasPrefix(locale, "ru")
	isMonth := plan == "month"

	var price int
	var title, description, label string
	if isRu {
		title, label = "SubGuard Премиум", "Премиум"
		if isMonth {
			price, description = settings.PriceStarsMonthRU, "Premium на 1 месяц"
		} else {
			price, description = settings.PriceStarsLifetimeRU, "Premium навсегда"
		}
	} else {
		title, label = "SubGuard Premium", "Premium"
		if isMonth {
			price, description = settings.PriceStarsMonthEN, "Premium for 1 month"
		} else {
			price, description = settings.PriceStarsLifetimeEN, "Premium forever"
		}
	}

	if price <= 0 {
		log.Printf("[payment.stars] invalid price=%d for locale=%s plan=%s", price, locale, plan)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "invalid price configuration"})
	}

	invoiceURL, err := h.bot.CreateInvoiceLink(c.Context(), &tgbot.CreateInvoiceLinkParams{
		Title:         title,
		Description:   description,
		Payload:       fmt.Sprintf("premium_stars_%s_%d", plan, user.ID),
		ProviderToken: "",
		Currency:      "XTR",
		Prices: []models.LabeledPrice{
			{Label: label, Amount: price},
		},
	})
	if err != nil {
		log.Printf("[payment.stars] CreateInvoiceLink error for user=%d: %v", user.ID, err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "invoice generation failed"})
	}

	return c.JSON(fiber.Map{"invoice_url": invoiceURL})
}

// ── Crypto Pay (@CryptoBot) ─────────────────────────────────────────
//
// The API base lives in config (cfg.CryptoPayAPIURL, trailing slash
// guaranteed) so we can point at the testnet or mainnet endpoint via
// the CRYPTO_PAY_API_URL env var without a rebuild.

type createInvoiceRequest struct {
	CurrencyType   string `json:"currency_type"`
	Fiat           string `json:"fiat"`
	Amount         string `json:"amount"`
	Description    string `json:"description"`
	Payload        string `json:"payload"`
	AllowComments  bool   `json:"allow_comments"`
	AllowAnonymous bool   `json:"allow_anonymous"`
}

type createInvoiceResponse struct {
	OK     bool `json:"ok"`
	Result struct {
		InvoiceID int64  `json:"invoice_id"`
		PayURL    string `json:"pay_url"`
	} `json:"result"`
}

// CreateCryptoInvoice generates a Crypto Pay (fiat/USD) payment link.
//
// POST /api/v1/payments/crypto
func (h *PaymentHandler) CreateCryptoInvoice(c fiber.Ctx) error {
	user := middleware.UserFromCtx(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}
	if h.cfg.CryptoPayToken == "" {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "crypto payments not configured"})
	}

	var settings model.AppSettings
	if err := h.db.FirstOrCreate(&settings, model.AppSettings{ID: 1}).Error; err != nil {
		log.Printf("[payment.crypto] settings read error: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "settings unavailable"})
	}

	var planBody planRequest
	_ = c.Bind().JSON(&planBody) // empty / missing body → lifetime default
	plan := normalizePlan(planBody.Plan)

	locale := user.Locale
	if locale == "" {
		locale = "en"
	}
	isRu := strings.HasPrefix(locale, "ru")
	isMonth := plan == "month"

	// Crypto pricing is locale-split (RU/EN) × plan, mirroring Stars.
	var price int
	if isRu {
		if isMonth {
			price = settings.PriceCryptoMonthUSDRU
		} else {
			price = settings.PriceCryptoLifetimeUSDRU
		}
	} else {
		if isMonth {
			price = settings.PriceCryptoMonthUSDEN
		} else {
			price = settings.PriceCryptoLifetimeUSDEN
		}
	}

	description := "SubGuard Premium"
	if isRu {
		description = "SubGuard Премиум"
	}

	if price <= 0 {
		log.Printf("[payment.crypto] invalid price=%d for plan=%s", price, plan)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "invalid price configuration"})
	}

	reqBody := createInvoiceRequest{
		CurrencyType:   "fiat",
		Fiat:           "USD",
		Amount:         strconv.Itoa(price),
		Description:    description,
		Payload:        fmt.Sprintf("premium_crypto_%s_%d", plan, user.ID),
		AllowComments:  false,
		AllowAnonymous: false,
	}

	bodyBytes, _ := json.Marshal(reqBody)
	req, err := http.NewRequestWithContext(c.Context(), http.MethodPost,
		h.cfg.CryptoPayAPIURL+"createInvoice", bytes.NewReader(bodyBytes))
	if err != nil {
		log.Printf("[payment.crypto] request build error: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "request build failed"})
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Crypto-Pay-API-Token", h.cfg.CryptoPayToken)

	resp, err := cryptoPayClient.Do(req)
	if err != nil {
		log.Printf("[payment.crypto] API call error: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "crypto api unavailable"})
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	log.Printf("[payment.crypto] API response status=%d body=%s", resp.StatusCode, string(respBody))

	if resp.StatusCode != http.StatusOK {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "crypto invoice failed"})
	}

	var invoiceResp createInvoiceResponse
	if err := json.Unmarshal(respBody, &invoiceResp); err != nil || !invoiceResp.OK {
		log.Printf("[payment.crypto] parse error or not ok: %v, ok=%v", err, invoiceResp.OK)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "crypto invoice parse failed"})
	}

	return c.JSON(fiber.Map{"invoice_url": invoiceResp.Result.PayURL})
}

// ── Crypto Pay Webhook ──────────────────────────────────────────────

type cryptoWebhookUpdate struct {
	UpdateType  string          `json:"update_type"`
	UpdateID    int64           `json:"update_id"`
	RequestDate string          `json:"request_date"`
	Payload     json.RawMessage `json:"payload"`
}

type cryptoInvoice struct {
	InvoiceID int64  `json:"invoice_id"`
	Status    string `json:"status"`
	Amount    string `json:"amount"`
	Fiat      string `json:"fiat"`
	PaidAt    string `json:"paid_at"`
	Payload   string `json:"payload"`
}

// HandleCryptoWebhook processes payment notifications from Crypto Pay.
// This endpoint is PUBLIC — no AuthMiddleware, no MaintenanceGuard.
// Security is enforced via HMAC-SHA256 signature verification.
//
// POST /webhook/crypto
func (h *PaymentHandler) HandleCryptoWebhook(c fiber.Ctx) error {
	if h.cfg.CryptoPayToken == "" {
		return c.Status(fiber.StatusServiceUnavailable).SendString("not configured")
	}

	rawBody := c.Body()
	signature := c.Get("crypto-pay-api-signature")

	// Signature: HMAC-SHA256(SHA256(token), body). Crypto Pay sends it as a
	// lower-case hex string. We decode to raw bytes BEFORE comparing so
	// hmac.Equal runs on equal-length inputs (32 bytes from SHA-256) and
	// stays constant-time; comparing hex strings of varying length leaks
	// the length of the supplied signature on a mismatch.
	tokenHash := sha256.Sum256([]byte(h.cfg.CryptoPayToken))
	mac := hmac.New(sha256.New, tokenHash[:])
	mac.Write(rawBody)

	sigBytes, decodeErr := hex.DecodeString(signature)
	if decodeErr != nil || !hmac.Equal(sigBytes, mac.Sum(nil)) {
		log.Printf("❌ [crypto/webhook] Invalid signature. Got=%q", signature)
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "invalid signature"})
	}

	var update cryptoWebhookUpdate
	if err := json.Unmarshal(rawBody, &update); err != nil {
		log.Printf("❌ [crypto/webhook] JSON parse error: %v", err)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "bad json"})
	}

	log.Printf("📥 [crypto/webhook] update_type=%s update_id=%d", update.UpdateType, update.UpdateID)

	if update.UpdateType != "invoice_paid" {
		return c.SendStatus(fiber.StatusOK)
	}

	var invoice cryptoInvoice
	if err := json.Unmarshal(update.Payload, &invoice); err != nil {
		log.Printf("❌ [crypto/webhook] invoice parse error: %v", err)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "bad invoice"})
	}

	log.Printf("📥 [crypto/webhook] invoice_id=%d status=%s payload=%q amount=%s %s",
		invoice.InvoiceID, invoice.Status, invoice.Payload, invoice.Amount, invoice.Fiat)

	// Payload: "premium_crypto_<plan>_<userID>" (legacy 3-part also OK).
	plan, userID, ok := parsePaymentPayload(invoice.Payload, "crypto")
	if !ok {
		log.Printf("❌ [crypto/webhook] unknown/malformed payload=%q, ignoring", invoice.Payload)
		return c.SendStatus(fiber.StatusOK)
	}
	log.Printf("✅ [crypto/webhook] Parsed plan=%s UserID=%d", plan, userID)

	// Idempotency: "crypto_<invoice_id>" in the shared charge ID column.
	// The previous flow did a pre-check SELECT for an existing donation
	// and then a tx with INSERT + UPDATE — a TOCTOU window in which two
	// concurrent webhooks (Telegram retry + scheduled retry, or two
	// pods racing on the same payload) both passed the SELECT and then
	// raced on the UNIQUE constraint inside their tx. The losing tx
	// rolled back with a 5xx, generating Sentry noise for what is
	// functionally a no-op.
	//
	// Replaced with INSERT ... ON CONFLICT (telegram_payment_charge_id)
	// DO NOTHING. The dedup is atomic at the row level: the first
	// concurrent caller inserts and flips premium in the same tx; every
	// subsequent caller sees RowsAffected == 0, skips the user UPDATE,
	// and returns 200. Audit Tier-1 #1.
	chargeID := fmt.Sprintf("crypto_%d", invoice.InvoiceID)

	var user model.User
	if err := h.db.First(&user, "id = ?", uint(userID)).Error; err != nil {
		log.Printf("❌ [crypto/webhook] user=%d not found: %v", userID, err)
		return c.SendStatus(fiber.StatusOK)
	}
	log.Printf("👤 [crypto/webhook] Found user: ID=%d, TelegramID=%d, IsDonator=%v",
		user.ID, user.TelegramID, user.IsDonator)

	// Convert via math.Round to avoid floating-point truncation: a
	// nominally "0.10" amount comes back from ParseFloat as
	// 0.099999999… and `int(x*100)` would silently store 9 cents
	// instead of 10. Crypto Pay USD invoices are always whole-cent
	// granularity, but guarding against IEEE-754 drift is cheap and
	// future-proofs us against fractional-fiat currencies. Audit Low.
	amountFloat, _ := strconv.ParseFloat(invoice.Amount, 64)
	amountCents := int(math.Round(amountFloat * 100))
	expiresAt := premiumExpiryFor(plan)

	var firstTime bool
	txErr := h.db.Transaction(func(tx *gorm.DB) error {
		donation := model.Donation{
			UserID:                  user.ID,
			TelegramID:              user.TelegramID,
			TelegramPaymentChargeID: chargeID,
			Amount:                  amountCents,
		}
		result := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "telegram_payment_charge_id"}},
			// idx_donations_charge_id is a PARTIAL unique index (WHERE
			// telegram_payment_charge_id <> ''); the conflict target must
			// repeat that predicate or Postgres rejects ON CONFLICT with
			// 42P10 — it finds no matching arbiter index.
			TargetWhere: clause.Where{Exprs: []clause.Expression{
				clause.Expr{SQL: "telegram_payment_charge_id <> ''"},
			}},
			DoNothing: true,
		}).Create(&donation)
		if result.Error != nil {
			return fmt.Errorf("create donation: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			// Already processed in an earlier (concurrent or retry) tx,
			// which already flipped is_donator. Nothing to do.
			return nil
		}
		firstTime = true
		return tx.Model(&model.User{}).
			Where("id = ?", user.ID).
			Updates(map[string]interface{}{
				"is_donator":         true,
				"premium_expires_at": expiresAt,
			}).Error
	})
	if txErr != nil {
		log.Printf("❌ [crypto/webhook] tx error for user=%d: %v", user.ID, txErr)
		return c.Status(fiber.StatusInternalServerError).SendString("tx error")
	}
	if !firstTime {
		log.Printf("⚠️ [crypto/webhook] Duplicate chargeID=%s for user=%d — skipping", chargeID, user.ID)
		return c.SendStatus(fiber.StatusOK)
	}
	log.Printf("🟢 [crypto/webhook] Premium activated for user=%d, invoice=%d", user.ID, invoice.InvoiceID)

	// Localized congratulation via Telegram
	if h.bot != nil {
		locale := user.Locale
		var text string
		if strings.HasPrefix(locale, "ru") {
			text = "🎉 <b>Спасибо за покупку!</b>\n\nPremium успешно активирован. Вернитесь в приложение, чтобы пользоваться всеми функциями!"
		} else {
			text = "🎉 <b>Thank you for your purchase!</b>\n\nPremium is activated. Return to the app to enjoy all features!"
		}

		log.Printf("✉️ [crypto/webhook] Sending congratulation to TG=%d", user.TelegramID)
		if _, err := h.bot.SendMessage(c.Context(), &tgbot.SendMessageParams{
			ChatID:    user.TelegramID,
			Text:      text,
			ParseMode: "HTML",
		}); err != nil {
			log.Printf("❌ [crypto/webhook] send error: %v", err)
		}
	}

	return c.SendStatus(fiber.StatusOK)
}
